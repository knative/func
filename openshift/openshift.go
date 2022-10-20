package openshift

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containers/storage/pkg/lockfile"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"

	"knative.dev/func/docker"
	"knative.dev/func/docker/creds"
	fnhttp "knative.dev/func/http"
	"knative.dev/func/k8s"
)

const (
	registryHost     = "image-registry.openshift-image-registry.svc"
	registryHostPort = registryHost + ":5000"
)

func GetServiceCA(ctx context.Context) (*x509.Certificate, error) {
	cliConf := k8s.GetClientConfig()
	restConf, err := cliConf.ClientConfig()
	if err != nil {
		return nil, err
	}

	// first try the cache
	si, err := loadServerInfo(restConf.Host)
	if err == nil && si.InternalCA != nil {
		return x509.ParseCertificate(si.InternalCA)
	}

	client, ns, err := k8s.NewClientAndResolvedNamespace("")
	if err != nil {
		return nil, err
	}

	cfgMapName := "service-ca-config-" + rand.String(5)

	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cfgMapName,
			Annotations: map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
		},
	}

	configMaps := client.CoreV1().ConfigMaps(ns)

	nameSelector := fields.OneTermEqualSelector("metadata.name", cfgMapName).String()
	listOpts := metav1.ListOptions{
		Watch:         true,
		FieldSelector: nameSelector,
	}

	watch, err := configMaps.Watch(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	defer watch.Stop()

	crtChan := make(chan string)
	go func() {
		for event := range watch.ResultChan() {
			cm, ok := event.Object.(*v1.ConfigMap)
			if !ok {
				continue
			}
			if crt, ok := cm.Data["service-ca.crt"]; ok {
				crtChan <- crt
				close(crtChan)
				break
			}
		}
	}()

	_, err = configMaps.Create(ctx, cfgMap, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = configMaps.Delete(ctx, cfgMapName, metav1.DeleteOptions{})
	}()

	select {
	case crt := <-crtChan:
		blk, _ := pem.Decode([]byte(crt))
		// save the CA to the cache
		_ = saveServerInfo(serverInfo{URL: restConf.Host, InternalCA: blk.Bytes})
		return x509.ParseCertificate(blk.Bytes)
	case <-time.After(time.Second * 5):
		return nil, errors.New("failed to get OpenShift's service CA in time")
	}
}

// WithOpenShiftServiceCA enables trust to OpenShift's service CA for internal image registry
func WithOpenShiftServiceCA() fnhttp.Option {
	var (
		err      error
		selectCA func(ctx context.Context, serverName string) (*x509.Certificate, error)
		ca       *x509.Certificate
		once     sync.Once
	)

	selectCA = func(ctx context.Context, serverName string) (*x509.Certificate, error) {
		if strings.HasPrefix(serverName, registryHost) {
			once.Do(func() {
				ca, err = GetServiceCA(ctx)
			})
			if err != nil {
				return nil, fmt.Errorf("cannot get CA: %w", err)
			}
			return ca, nil
		}
		return nil, nil
	}

	return fnhttp.WithSelectCA(selectCA)
}

func GetDefaultRegistry() string {
	ns, _ := k8s.GetNamespace("")
	if ns == "" {
		ns = "default"
	}

	return registryHostPort + "/" + ns
}

func GetDockerCredentialLoaders() []creds.CredentialsCallback {
	conf := k8s.GetClientConfig()

	rawConf, err := conf.RawConfig()
	if err != nil {
		return nil
	}

	cc, ok := rawConf.Contexts[rawConf.CurrentContext]
	if !ok {
		return nil
	}
	authInfo := rawConf.AuthInfos[cc.AuthInfo]

	credentials := docker.Credentials{
		Username: "openshift",
		Password: authInfo.Token,
	}

	return []creds.CredentialsCallback{
		func(registry string) (docker.Credentials, error) {
			if registry == registryHostPort {
				return credentials, nil
			}
			return docker.Credentials{}, creds.ErrCredentialsNotFound
		},
	}

}

var isOpenShift bool
var checkOpenShiftOnce sync.Once

func IsOpenShift() bool {
	checkOpenShiftOnce.Do(func() {
		cliConfig := k8s.GetClientConfig()
		restConf, err := cliConfig.ClientConfig()

		if err != nil {
			return
		}

		// first check the cache
		_, err = loadServerInfo(restConf.Host)
		if err == nil {
			isOpenShift = true
			return
		}

		client, err := kubernetes.NewForConfig(restConf)
		if err != nil {
			return
		}

		_, err = client.CoreV1().Services("openshift-image-registry").Get(context.TODO(), "image-registry", metav1.GetOptions{})
		if err == nil || k8sErrors.IsForbidden(err) {
			isOpenShift = true
			// save the server to the cache
			_ = saveServerInfo(serverInfo{URL: restConf.Host})
			return
		}
	})
	return isOpenShift
}

const cacheVersion = 1

type serverInfo struct {
	URL        string    `json:"url"`
	LastUpdate time.Time `json:"last_update"`
	InternalCA []byte    `json:"internal_ca"`
}

type cache struct {
	Version int                   `json:"version"`
	Servers map[string]serverInfo `json:"servers"`
}

var errNotFound = errors.New("not found")

func cacheFilePath() string {
	var cacheFile string
	if cacheDir, ok := os.LookupEnv("XDG_CACHE_HOME"); ok {
		cacheFile = filepath.Join(cacheDir, "func", "openshift-data.json")
	} else if home, err := os.UserHomeDir(); err == nil {
		cacheFile = filepath.Join(home, ".cache", "func", "openshift-data.json")
	}
	return cacheFile
}

func saveServerInfo(si serverInfo) error {
	cacheFile := cacheFilePath()
	l, err := lockfile.GetLockfile(cacheFile)
	if err != nil {
		return fmt.Errorf("cannot lock cache file: %w", err)
	}
	l.Lock()
	defer l.Unlock()

	c, err := loadCache()
	if err != nil {
		return fmt.Errorf("cannot load cache: %w", err)
	}

	si.LastUpdate = time.Now()
	c.Servers[si.URL] = si

	err = saveCache(c)
	if err != nil {
		return fmt.Errorf("cannot save cache: %w", err)
	}

	return nil
}

func loadServerInfo(url string) (s serverInfo, err error) {
	l, err := lockfile.GetLockfile(cacheFilePath())
	if err != nil {
		return serverInfo{}, fmt.Errorf("cannot get cache lock: %w", err)
	}
	l.RLock()
	defer l.Unlock()

	c, err := loadCache()
	if err != nil {
		return serverInfo{}, fmt.Errorf("cannot load catche: %w", err)
	}

	if si, ok := c.Servers[url]; ok {
		if time.Since(si.LastUpdate) > time.Hour {
			return serverInfo{}, errNotFound
		}
		return si, nil
	}
	return serverInfo{}, errNotFound
}

func loadCache() (c cache, err error) {
	bs, err := os.ReadFile(cacheFilePath())
	if err != nil {
		return c, fmt.Errorf("cannot read cache file: %w", err)
	}
	if len(bs) != 0 {
		err = json.Unmarshal(bs, &c)
		if err != nil {
			return c, fmt.Errorf("cannot unarshal cache file: %w", err)
		}
	}
	if c.Version != cacheVersion {
		// version mismatch, re-init the struct
		c.Version = cacheVersion
		c.Servers = make(map[string]serverInfo, 1)
	}
	return c, nil
}

func saveCache(c cache) error {
	bs, err := json.Marshal(&c)
	if err != nil {
		return fmt.Errorf("cannot marshal cache data: %w", err)
	}
	err = os.WriteFile(cacheFilePath(), bs, 0644)
	if err != nil {
		return fmt.Errorf("cannot write cache file: %w", err)
	}
	return nil
}
