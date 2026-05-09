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
	fn "knative.dev/func/pkg/functions"
	k8s "knative.dev/func/pkg/k8s"
	"knative.dev/pkg/ptr"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
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
		{
			name: "refer non-existent secret",
			core: fake.NewClientset(&corev1.ServiceAccount{
				ObjectMeta:       metav1.ObjectMeta{Name: "default", Namespace: "default"},
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "non-existent"}},
			}).CoreV1(),
			image: imageShortName,
			errPred: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "not found")
			},
		},
		{
			name:  "only incorrect credentials in secret",
			core:  core(createSecret(securedRegistry, username, "incorrect", corev1.SecretTypeDockercfg)),
			image: securedImage,
			errPred: func(err error) bool {
				return errors.Is(err, errOnlyIncorrectPullSecretFound)
			},
		},
		{
			name:  "broken server",
			core:  core(),
			image: brokenRegistry + "/" + imageShortName,
			errPred: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), " 500 ")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = checkPullPermissions(t.Context(), tt.core, trans, tt.image, "default", "")
			p := tt.errPred
			if p == nil {
				p = func(err error) bool {
					return err == nil
				}
			}
			if !p(err) {
				t.Errorf("unexpected return value: %v", err)
			}
		})
	}

}

func TestCheckPullPermissions_FunctionImagePullSecret(t *testing.T) {
	secretA := createSecret(securedRegistry,
		username, password,
		corev1.SecretTypeDockerConfigJson,
	)

	trans := setupRegistry(t)

	t.Run("function-level secret with correct credentials", func(t *testing.T) {
		// The secret is registered in the cluster but NOT on the ServiceAccount.
		// It's provided as a function-level image pull secret.
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		}
		coreClient := fake.NewClientset(sa, secretA).CoreV1()

		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", secretA.Name)
		if err != nil {
			t.Errorf("expected no error with valid function-level pull secret, got: %v", err)
		}
	})

	t.Run("function-level secret with incorrect credentials", func(t *testing.T) {
		badSecret := createSecret(securedRegistry, username, "wrong-password", corev1.SecretTypeDockerConfigJson)
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		}
		coreClient := fake.NewClientset(sa, badSecret).CoreV1()

		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", badSecret.Name)
		if !errors.Is(err, errOnlyIncorrectPullSecretFound) {
			t.Errorf("expected errOnlyIncorrectPullSecretFound, got: %v", err)
		}
	})

	t.Run("no function-level secret falls back to SA", func(t *testing.T) {
		coreClient := core(secretA)
		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", "")
		if err != nil {
			t.Errorf("expected no error when SA has valid pull secret, got: %v", err)
		}
	})
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

// TestUpdateService_EnvsPropagated checks that the updateService closure replaces
// (not merges) the container env list with the supplied newEnv slice.
// It calls updateService directly — it does not exercise Deploy, ProcessEnvs, or
// UpdateServiceWithRetry.
func TestUpdateService_EnvsPropagated(t *testing.T) {
	// Simulate an existing deployed service that carries a stale env var.
	previousService := &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-func",
			Namespace: "default",
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "example.com/test:v1",
									Env:   []corev1.EnvVar{{Name: "OLD_VAR", Value: "old"}},
								},
							},
						},
					},
				},
			},
		},
	}

	f := fn.Function{
		Name: "test-func",
		Deploy: fn.DeploySpec{
			Image: "example.com/test:v2",
		},
	}

	// newEnv represents the resolved env vars the deployer computes via
	// k8s.ProcessEnvs(f.Run.Envs, ...) before invoking updateService.
	newEnv := []corev1.EnvVar{
		{Name: "MYVAR", Value: "myvalue"},
		{Name: "OTHER", Value: "othervalue"},
	}

	// Obtain the update closure — this mirrors what Deploy() does when
	// previousService != nil (the service already exists on the cluster).
	updateFn := updateService(f, previousService, newEnv, nil, nil, nil, nil, false)

	// Apply the closure to a working copy of the service (mirroring what
	// UpdateServiceWithRetry does internally on each attempt).
	svcCopy := previousService.DeepCopy()
	result, err := updateFn(svcCopy)
	if err != nil {
		t.Fatalf("updateService closure returned unexpected error: %v", err)
	}

	containers := result.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	gotEnv := containers[0].Env
	envMap := make(map[string]string, len(gotEnv))
	for _, e := range gotEnv {
		envMap[e.Name] = e.Value
	}

	if v, ok := envMap["MYVAR"]; !ok || v != "myvalue" {
		t.Errorf("expected MYVAR=myvalue in updated container env, got: %v", gotEnv)
	}
	if v, ok := envMap["OTHER"]; !ok || v != "othervalue" {
		t.Errorf("expected OTHER=othervalue in updated container env, got: %v", gotEnv)
	}
	// OLD_VAR must not survive: updateService replaces (not merges) the env list.
	if _, ok := envMap["OLD_VAR"]; ok {
		t.Errorf("expected OLD_VAR to be replaced by updateService but it was still present: %v", gotEnv)
	}
}

// TestGenerateNewService_ResourceSetsPopulated is a regression test for the
// create (first-deploy) path. It proves that generateNewService populates the
// tracker's References so that the subsequent CheckResourcesArePresent call in
// Deploy() actually validates them.
func TestGenerateNewService_ResourceSetsPopulated(t *testing.T) {
	secretName := "my-secret"
	configMapName := "my-configmap"

	f := fn.Function{
		Name: "test-func",
		Deploy: fn.DeploySpec{
			Image: "example.com/test:v1",
		},
	}
	f.Run.Envs.Add("FROM_SECRET", "{{ secret:"+secretName+":key }}")
	f.Run.Envs.Add("FROM_CM", "{{ configMap:"+configMapName+":key }}")

	tracker := k8s.NewTracker()

	_, err := generateNewService(f, nil, false, tracker)
	if err != nil {
		t.Fatalf("generateNewService returned unexpected error: %v", err)
	}

	if !tracker.Secrets.Has(secretName) {
		t.Errorf("expected tracker.Secrets to contain %q after generateNewService, got: %v", secretName, tracker.Secrets)
	}
	if !tracker.ConfigMaps.Has(configMapName) {
		t.Errorf("expected tracker.ConfigMaps to contain %q after generateNewService, got: %v", configMapName, tracker.ConfigMaps)
	}
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
