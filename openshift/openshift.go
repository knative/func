package openshift

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"

	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/docker/creds"
	fnhttp "knative.dev/kn-plugin-func/http"
	"knative.dev/kn-plugin-func/k8s"
)

const (
	registryHost     = "image-registry.openshift-image-registry.svc"
	registryHostPort = registryHost + ":5000"
)

func GetServiceCA(ctx context.Context) (*x509.Certificate, error) {
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
		return x509.ParseCertificate(blk.Bytes)
	case <-time.After(time.Second * 5):
		return nil, errors.New("failed to get OpenShift's service CA in time")
	}
}

// WithOpenShiftServiceCA enables trust to OpenShift's service CA for internal image registry
func WithOpenShiftServiceCA() fnhttp.Option {
	var selectCA func(ctx context.Context, serverName string) (*x509.Certificate, error)
	ca, err := GetServiceCA(context.TODO())
	if err == nil {
		selectCA = func(ctx context.Context, serverName string) (*x509.Certificate, error) {
			if strings.HasPrefix(serverName, registryHost) {
				return ca, nil
			}
			return nil, nil
		}
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

	var user string
	parts := strings.SplitN(cc.AuthInfo, "/", 2)
	if len(parts) >= 1 {
		user = parts[0]
	} else {
		return nil
	}
	credentials := docker.Credentials{
		Username: user,
		Password: authInfo.Token,
	}

	return []creds.CredentialsCallback{
		func(registry string) (docker.Credentials, error) {
			if registry == registryHostPort {
				return credentials, nil
			}
			return docker.Credentials{}, nil
		},
	}

}

// FIXME(lkingland):
// Using this global variable meaans any processing importing this package
// will be "locked" to the value of the cluster to which it is attached on the
// first call.  This breaks the ability to have multiple instances of the Client
// which change their target cluster during execution.
var isOpenShift bool
var checkOpenShiftOnce sync.Once

func IsOpenShift() bool {
	checkOpenShiftOnce.Do(func() {
		isOpenShift = false // FIXME(lkingland):  False is the zero value.
		client, err := k8s.NewKubernetesClientset()
		if err != nil {
			// FIXME(lkingland): This should perhaps print the err to stderr, but I
			// suspect it would be better to bubble this error.  The caller probably
			// would like to know the system was unable to determine if the target
			// cluster is OpenShift, since a fair bit of logic relies on it.
			isOpenShift = false
			return
		}
		_, err = client.CoreV1().Services("openshift-image-registry").Get(context.TODO(), "image-registry", metav1.GetOptions{})
		if err == nil || k8sErrors.IsForbidden(err) {
			isOpenShift = true
			return
		}
		// FIXME(lkingland): This logic seems to obscure legitimate errors, treating
		// them as equivalent to IsOpenShift==true.
		// For example when no cluster is available, this fails with a connection
		// refused error, leading to the system state of presuming we are in
		// OpenShift mode.
		// This may be improved by adopting a different detection mechanism, or at
		// least.
		// See [associated discussion](https://github.com/knative-sandbox/kn-plugin-func/pull/825#discussion_r811520395)
		// See [appropos issue](https://github.com/knative-sandbox/kn-plugin-func/issues/867)
	})
	return isOpenShift
}
