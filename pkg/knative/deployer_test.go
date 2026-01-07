package knative

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/go-connections/sockets"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"knative.dev/pkg/ptr"
)

const (
	username        = "developer"
	password        = "iddqd"
	publicRegistry  = "public-registry.local"
	securedRegistry = "secured-registry.local"
	brokenRegistry  = "internal-error.local"
	imageShortName  = "project/img"
	publicImage     = publicRegistry + "/" + imageShortName
	securedImage    = securedRegistry + "/" + imageShortName
)

func TestCheckPullPermissions(t *testing.T) {
	var err error

	secretA := createSecret(securedRegistry,
		username, password,
		corev1.SecretTypeDockerConfigJson,
	)

	secretB := createSecret("https://"+securedRegistry,
		username, password,
		corev1.SecretTypeDockercfg,
	)

	secretC := createSecret("https://index.docker.io/v1/",
		username, password,
		corev1.SecretTypeDockercfg,
	)

	secretD := createSecret("index.docker.io",
		username, password,
		corev1.SecretTypeDockercfg,
	)

	secretE := createSecret("docker.io",
		username, password,
		corev1.SecretTypeDockercfg,
	)

	trans := setupRegistry(t)

	tests := []struct {
		name    string
		core    v1.CoreV1Interface
		image   string
		errPred func(error) bool
	}{
		{
			name:  "public image",
			core:  core(),
			image: publicImage,
		},
		{
			name:  "creds in dockerconfigjson",
			core:  core(secretA),
			image: securedImage,
		},
		{
			name:  "creds in dockercfg",
			core:  core(secretB),
			image: securedImage,
		},
		{
			name:  "multiple secrets",
			core:  core(secretC, secretA, secretD),
			image: securedImage,
		},
		{
			name:  "missing creds",
			core:  core(),
			image: securedImage,
			errPred: func(err error) bool {
				return errors.Is(err, errPullSecretNotFound)
			},
		},
		{
			name:  "broken server",
			core:  core(),
			image: brokenRegistry + "/" + imageShortName,
			errPred: func(err error) bool {
				if err == nil {
					return false
				}
				return strings.Contains(err.Error(), " 500 ")
			},
		},
		{
			name:  "dockerhub as https://index.docker.io/v1/",
			core:  core(secretC),
			image: imageShortName,
		},
		{
			name:  "dockerhub as index.docker.io",
			core:  core(secretD),
			image: imageShortName,
		},
		{
			name:  "dockerhub as docker.io",
			core:  core(secretE),
			image: imageShortName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = checkPullPermissions(t.Context(), tt.core, trans, tt.image, "default")
			p := tt.errPred
			if p == nil {
				p = func(err error) bool {
					return err == nil
				}
			}
			if !p(err) {
				t.Error("unexpected return value")
			}
		})
	}

}

var secretCounter int32

func createSecret(reg, uname, pwd string, typ corev1.SecretType) *corev1.Secret {
	var cf = configfile.ConfigFile{
		AuthConfigs: map[string]types.AuthConfig{
			reg: {Auth: base64.StdEncoding.EncodeToString([]byte(uname + ":" + pwd))},
		},
	}

	var (
		data    []byte
		dataKey string
		err     error
	)

	switch typ {
	case corev1.SecretTypeDockerConfigJson:
		dataKey = corev1.DockerConfigJsonKey
		data, err = json.Marshal(&cf)
		if err != nil {
			panic(err)
		}
	case corev1.SecretTypeDockercfg:
		dataKey = corev1.DockerConfigKey
		data, err = json.Marshal(&cf.AuthConfigs)
		if err != nil {
			panic(err)
		}
	default:
		panic("unsupported type: " + typ)
	}
	n := fmt.Sprintf("secret-%d", atomic.AddInt32(&secretCounter, 1))
	s := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: "default"},
		Immutable:  ptr.Bool(true),
		Data:       map[string][]byte{dataKey: data},
		Type:       typ,
	}

	return s
}

func core(scs ...*corev1.Secret) v1.CoreV1Interface {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	sa.ImagePullSecrets = make([]corev1.LocalObjectReference, 0, len(scs))

	var objs = make([]runtime.Object, 1, len(scs)+1)
	objs[0] = sa
	for _, sc := range scs {
		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: sc.Name})
		objs = append(objs, sc)
	}
	return fake.NewClientset(objs...).CoreV1()
}

func setupRegistry(t *testing.T) http.RoundTripper {
	t.Helper()

	l := sockets.NewInmemSocket("test", 1024)
	t.Cleanup(func() {
		_ = l.Close()
	})

	dockerHubRegistry := "index.docker.io"

	validHosts := map[string]struct{}{
		publicRegistry:    {},
		securedRegistry:   {},
		brokenRegistry:    {},
		dockerHubRegistry: {},
	}
	dialContext := func(_ context.Context, network, addr string) (net.Conn, error) {
		h, _, _ := net.SplitHostPort(addr)
		if _, ok := validHosts[h]; !ok {
			return nil, fmt.Errorf("invalid host: %q", h)
		}
		return l.Dial(network, addr)
	}

	trans := &http.Transport{
		DialContext:    dialContext,
		DialTLSContext: dialContext,
	}

	reg := registry.New(registry.Logger(log.New(io.Discard, "", 0)))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case publicRegistry:
		case securedRegistry, dockerHubRegistry:
			if !assertAuth(username, password, w, r) {
				return
			}
		case brokenRegistry:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("broken server"))
			return
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad host"))
			return
		}
		reg.ServeHTTP(w, r)
	})
	server := http.Server{Handler: handler}

	go func() {
		_ = server.Serve(l)
	}()

	err := remote.Push(
		name.MustParseReference(publicImage),
		empty.Image,
		remote.WithTransport(trans),
	)
	if err != nil {
		t.Fatal(err)
	}

	return trans
}

func assertAuth(uname, pwd string, w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if ok && user == uname && pass == pwd {
		return true
	}
	w.Header().Add("WWW-Authenticate", "basic")
	w.WriteHeader(401)
	_, _ = fmt.Fprintln(w, "Unauthorised.")
	return false
}
