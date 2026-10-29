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
	"time"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/go-connections/sockets"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	fn "knative.dev/func/pkg/functions"
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

	secretF := createSecret("https://docker.io",
		username, password,
		corev1.SecretTypeDockercfg,
	)

	secretG := createSecret("index.docker.io/v1/",
		username, password,
		corev1.SecretTypeDockercfg,
	)

	secretH := createSecret("docker.io",
		username, password,
		corev1.SecretTypeDockerConfigJson,
	)

	// secretI and secretJ have an empty entry under the canonical key and
	// credentials under docker.io. docker login writes such an empty entry to
	// ~/.docker/config.json when a credential store keeps the password, and a
	// dockerconfigjson secret can be made from that file. secretI holds the
	// entries in the older dockercfg format.
	secretI := createSecret("docker.io",
		username, password,
		corev1.SecretTypeDockercfg,
	)
	secretI.Data[corev1.DockerConfigKey] = fmt.Appendf(nil,
		`{%q: {}, "docker.io": {"auth": %q}}`, authn.DefaultAuthKey,
		base64.StdEncoding.EncodeToString([]byte(username+":"+password)))

	// secretJ is the dockerconfigjson form. Loading it sets a server address
	// on the empty entry, which must not count as credentials.
	secretJ := createSecret("docker.io",
		username, password,
		corev1.SecretTypeDockerConfigJson,
	)
	secretJ.Data[corev1.DockerConfigJsonKey] = fmt.Appendf(nil,
		`{"auths": {%q: {}, "docker.io": {"auth": %q}}}`, authn.DefaultAuthKey,
		base64.StdEncoding.EncodeToString([]byte(username+":"+password)))

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
			name:  "dockerhub as https://docker.io",
			core:  core(secretF),
			image: imageShortName,
		},
		{
			name:  "dockerhub as index.docker.io/v1/",
			core:  core(secretG),
			image: imageShortName,
		},
		{
			name:  "dockerhub as docker.io in dockerconfigjson",
			core:  core(secretH),
			image: imageShortName,
		},
		{
			name:  "dockerhub as docker.io next to an empty canonical entry",
			core:  core(secretI),
			image: imageShortName,
		},
		{
			name:  "dockerhub as docker.io next to an empty canonical entry in dockerconfigjson",
			core:  core(secretJ),
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
			err = checkPullPermissions(t.Context(), tt.core, trans, tt.image, "default", "", false)
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

// TestCanonicalizeDockerHubAuth ensures a Docker Hub entry stored under any
// spelling ends up under the key docker/cli looks up, and that the choice
// between several entries does not depend on map order.
func TestCanonicalizeDockerHubAuth(t *testing.T) {
	tests := []struct {
		name  string
		auths map[string]string // key to username
		want  string            // username under authn.DefaultAuthKey, "" for none
	}{
		{
			name:  "canonical key wins",
			auths: map[string]string{authn.DefaultAuthKey: "canonical", "https://index.docker.io": "other"},
			want:  "canonical",
		},
		{
			name:  "empty canonical entry does not win",
			auths: map[string]string{authn.DefaultAuthKey: "", "docker.io": "bare"},
			want:  "bare",
		},
		{
			name:  "empty entries skipped",
			auths: map[string]string{"https://index.docker.io": "", "docker.io": "bare"},
			want:  "bare",
		},
		{
			name:  "index.docker.io host before docker.io host",
			auths: map[string]string{"docker.io": "docker", "https://index.docker.io": "index"},
			want:  "index",
		},
		{
			name:  "same host in sorted order",
			auths: map[string]string{"https://docker.io": "https", "docker.io": "bare"},
			want:  "bare",
		},
		{
			name:  "other registries untouched",
			auths: map[string]string{"quay.io": "quay"},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := &configfile.ConfigFile{AuthConfigs: map[string]types.AuthConfig{}}
			for key, user := range tt.auths {
				cf.AuthConfigs[key] = types.AuthConfig{Username: user}
			}
			canonicalizeDockerHubAuth(cf)
			if got := cf.AuthConfigs[authn.DefaultAuthKey].Username; got != tt.want {
				t.Errorf("username under %q = %q, want %q", authn.DefaultAuthKey, got, tt.want)
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

		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", secretA.Name, false)
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

		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", badSecret.Name, false)
		if !errors.Is(err, errOnlyIncorrectPullSecretFound) {
			t.Errorf("expected errOnlyIncorrectPullSecretFound, got: %v", err)
		}
	})

	t.Run("no function-level secret falls back to SA", func(t *testing.T) {
		coreClient := core(secretA)
		err := checkPullPermissions(t.Context(), coreClient, trans, securedImage, "default", "", false)
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
// caller-supplied referencedSecrets and referencedConfigMaps sets so that the
// subsequent CheckResourcesArePresent call in Deploy() actually validates them.
// Before the fix, generateNewService allocated its own internal sets and the
// caller's sets remained empty, causing validation to be silently skipped.
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

	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	_, err := generateNewService(f, nil, false, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err != nil {
		t.Fatalf("generateNewService returned unexpected error: %v", err)
	}

	if !referencedSecrets.Has(secretName) {
		t.Errorf("expected referencedSecrets to contain %q after generateNewService, got: %v", secretName, referencedSecrets)
	}
	if !referencedConfigMaps.Has(configMapName) {
		t.Errorf("expected referencedConfigMaps to contain %q after generateNewService, got: %v", configMapName, referencedConfigMaps)
	}
}

// TestGenerateNewService_EnvsPropagated is a regression test for SRVOCF-650.
// It verifies that plain environment variables set in f.Run.Envs appear on the
// generated Knative service's container spec when deploying a new function.
func TestGenerateNewService_EnvsPropagated(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Deploy: fn.DeploySpec{
			Image: "example.com/test:v1",
		},
	}
	f.Run.Envs.Add("SLACK_AUTH_TOKEN", "my-token")
	f.Run.Envs.Add("OTHER_VAR", "other-value")

	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	svc, err := generateNewService(f, nil, false, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err != nil {
		t.Fatalf("generateNewService returned unexpected error: %v", err)
	}

	containers := svc.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	envMap := make(map[string]string, len(containers[0].Env))
	for _, e := range containers[0].Env {
		envMap[e.Name] = e.Value
	}

	if v, ok := envMap["SLACK_AUTH_TOKEN"]; !ok || v != "my-token" {
		t.Errorf("expected SLACK_AUTH_TOKEN=my-token in container env, got: %v", containers[0].Env)
	}
	if v, ok := envMap["OTHER_VAR"]; !ok || v != "other-value" {
		t.Errorf("expected OTHER_VAR=other-value in container env, got: %v", containers[0].Env)
	}
}

// TestDeploy_RejectsKafka verifies the knative deployer refuses a function
// configured with run.kafka: the Kafka runtime sidecar is injected only by the
// raw and keda deployers, and the in-process path is retired, so deploying on
// knative would silently produce a function that never receives records. The
// rejection happens before any cluster client is created, so no cluster is
// needed here.
func TestDeploy_RejectsKafka(t *testing.T) {
	d := NewDeployer()
	f := fn.Function{
		Name:      "test-func",
		Namespace: "default", // set so onClusterFix does not query the cluster
		Deploy:    fn.DeploySpec{Image: "example.com/test:v1"},
	}
	f.Run.Kafka = &fn.KafkaConfig{
		Brokers:       "kafka:9092",
		Topic:         "orders",
		ConsumerGroup: "test-func",
	}

	_, err := d.Deploy(context.Background(), f)
	if err == nil {
		t.Fatal("expected Deploy to reject a function with run.kafka, got nil error")
	}
	if !strings.Contains(err.Error(), "not supported on the knative deployer") {
		t.Errorf("expected an unsupported-on-knative-deployer error, got: %v", err)
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

func TestWaitForReadyOrPrivateRegistry_Success(t *testing.T) {
	waitForService := func(context.Context) error {
		return nil // returns immediately with success
	}
	isPrivateRegistry := func(context.Context) bool {
		return false
	}
	err := waitForReadyOrPrivateRegistry(t.Context(), waitForService, isPrivateRegistry, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWaitForReadyOrPrivateRegistry_WaitError(t *testing.T) {
	expectedErr := errors.New("timeout waiting for service")
	waitForService := func(context.Context) error {
		return expectedErr // returns immediately with error
	}
	isPrivateRegistry := func(context.Context) bool {
		return false
	}
	err := waitForReadyOrPrivateRegistry(t.Context(), waitForService, isPrivateRegistry, 10*time.Millisecond)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestWaitForReadyOrPrivateRegistry_PrivateRegistry(t *testing.T) {
	waitCanceled := make(chan struct{})
	waitForService := func(ctx context.Context) error {
		<-ctx.Done()
		close(waitCanceled)
		return ctx.Err()
	}
	isPrivate := false
	isPrivateRegistry := func(context.Context) bool {
		isPrivate = true
		return true // instantly return true
	}
	err := waitForReadyOrPrivateRegistry(t.Context(), waitForService, isPrivateRegistry, 10*time.Millisecond)
	if !errors.Is(err, errPrivateRegistry) {
		t.Fatalf("expected errPrivateRegistry, got %v", err)
	}
	if !isPrivate {
		t.Fatalf("expected isPrivateRegistry to be called")
	}
	select {
	case <-waitCanceled:
	case <-time.After(time.Second):
		t.Fatal("expected waitForService context to be canceled")
	}
}

func TestWaitForReadyOrPrivateRegistry_DelayedSuccess(t *testing.T) {
	waitForService := func(context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}
	isPrivateRegistry := func(context.Context) bool {
		return false // never private
	}
	err := waitForReadyOrPrivateRegistry(t.Context(), waitForService, isPrivateRegistry, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// The waiter must have left waitForService before we return. Cancel alone is
// not enough: a goroutine can still be unwinding after Deploy has returned.
func TestWaitForReadyOrPrivateRegistry_PrivateRegistryJoinsWaiter(t *testing.T) {
	var exited atomic.Bool
	waitForService := func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(150 * time.Millisecond)
		exited.Store(true)
		return ctx.Err()
	}
	isPrivateRegistry := func(context.Context) bool {
		return true
	}
	err := waitForReadyOrPrivateRegistry(t.Context(), waitForService, isPrivateRegistry, time.Millisecond)
	if !errors.Is(err, errPrivateRegistry) {
		t.Fatalf("expected errPrivateRegistry, got %v", err)
	}
	if !exited.Load() {
		t.Fatal("waitForService still running after return")
	}
}

func TestWaitForReadyOrPrivateRegistry_CallerCancelJoinsWaiter(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	var exited atomic.Bool
	waitForService := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		time.Sleep(150 * time.Millisecond)
		exited.Store(true)
		return ctx.Err()
	}
	isPrivateRegistry := func(context.Context) bool {
		return false
	}
	go func() {
		<-started
		cancel()
	}()
	err := waitForReadyOrPrivateRegistry(ctx, waitForService, isPrivateRegistry, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !exited.Load() {
		t.Fatal("waitForService still running after caller cancel")
	}
}

func TestSyncKnativeTriggers_DeletesOwnedStaleTrigger(t *testing.T) {
	ksvc := testKnativeService("my-func")
	desired := map[string]*eventingv1.Trigger{
		"my-func-function-trigger-0": newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "default"}, 0),
	}
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 2)

	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(&eventingv1.TriggerList{Items: []eventingv1.Trigger{*existing}}, nil)
	client.Recorder().DeleteTrigger("my-func-function-trigger-2", nil)

	if err := syncKnativeTriggers(t.Context(), client, ksvc, desired); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	client.Recorder().Validate()
}

func TestSyncKnativeTriggers_UpdatesChangedDesiredTrigger(t *testing.T) {
	ksvc := testKnativeService("my-func")
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old-broker"}, 0)
	delete(existing.Annotations, knativeTriggerManagedByAnnotation)
	desired := map[string]*eventingv1.Trigger{
		"my-func-function-trigger-0": newKnativeTrigger(ksvc, fn.KnativeSubscription{
			Source:  "new-broker",
			Filters: map[string]string{"type": "dev.knative.func"},
		}, 0),
	}

	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(&eventingv1.TriggerList{Items: []eventingv1.Trigger{*existing}}, nil)
	client.Recorder().UpdateTrigger(func(t *testing.T, got any) {
		trigger := got.(*eventingv1.Trigger)
		if trigger.Name != "my-func-function-trigger-0" {
			t.Fatalf("expected trigger name my-func-function-trigger-0, got %q", trigger.Name)
		}
		if trigger.Spec.Broker != "new-broker" {
			t.Fatalf("expected broker new-broker, got %q", trigger.Spec.Broker)
		}
		if trigger.Spec.Filter.Attributes["type"] != "dev.knative.func" {
			t.Fatalf("expected updated filter attributes, got %v", trigger.Spec.Filter.Attributes)
		}
		if trigger.Annotations[knativeTriggerManagedByAnnotation] != knativeTriggerManagedByValue {
			t.Fatalf("expected managed annotation, got %v", trigger.Annotations)
		}
	}, nil)

	if err := syncKnativeTriggers(t.Context(), client, ksvc, desired); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	client.Recorder().Validate()
}

func TestSyncKnativeTriggers_SkipsUnownedSamePrefixTrigger(t *testing.T) {
	ksvc := testKnativeService("my-func")
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 2)
	existing.Annotations = nil
	existing.OwnerReferences = nil

	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(&eventingv1.TriggerList{Items: []eventingv1.Trigger{*existing}}, nil)

	if err := syncKnativeTriggers(t.Context(), client, ksvc, nil); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	client.Recorder().Validate()
}

func TestSyncKnativeTriggers_EventingUnavailable(t *testing.T) {
	ksvc := testKnativeService("my-func")
	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(nil, errors.New("no or newer Knative Eventing API found on the backend"))

	if err := syncKnativeTriggers(t.Context(), client, ksvc, nil); err != nil {
		t.Fatalf("expected eventing unavailable to be ignored, got %v", err)
	}
	client.Recorder().Validate()
}

func TestSyncKnativeTriggers_DeleteError(t *testing.T) {
	ksvc := testKnativeService("my-func")
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 5)
	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(&eventingv1.TriggerList{Items: []eventingv1.Trigger{*existing}}, nil)
	client.Recorder().DeleteTrigger("my-func-function-trigger-5", errors.New("connection refused"))

	err := syncKnativeTriggers(t.Context(), client, ksvc, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected 'connection refused' in error, got: %v", err)
	}
	client.Recorder().Validate()
}

func testKnativeService(name string) *servingv1.Service {
	return &servingv1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.knative.dev/v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  "ksvc-uid",
		},
	}
}

func TestIsKnativeFunctionTriggerName(t *testing.T) {
	cases := []struct {
		service, name string
		want          bool
	}{
		{service: "my-func", name: "my-func-function-trigger-0", want: true},
		{service: "my-func", name: "my-func-function-trigger-10", want: true},
		{service: "my-func", name: "my-func-function-trigger-01", want: false},
		{service: "my-func", name: "my-func-function-trigger-backup", want: false},
		{service: "my-func", name: "my-func-function-trigger-extra-function-trigger-0", want: false},
		{service: "my-func", name: "my-func-function-trigger-", want: false},
		{service: "foo", name: "foobar-function-trigger-0", want: false},
		{service: "", name: "-function-trigger-0", want: false},
	}
	for _, tc := range cases {
		if got := isKnativeFunctionTriggerName(tc.service, tc.name); got != tc.want {
			t.Errorf("isKnativeFunctionTriggerName(%q, %q) = %v, want %v", tc.service, tc.name, got, tc.want)
		}
	}
}

func TestSyncKnativeTriggers_RemoveSubscriptionKeepsForeign(t *testing.T) {
	ksvc := testKnativeService("my-func")
	keep := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "default"}, 0)
	stale := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 2)
	similar := &eventingv1.Trigger{
		ObjectMeta: metav1.ObjectMeta{Name: "my-func-function-trigger-backup"},
		Spec:       eventingv1.TriggerSpec{Broker: "user-broker"},
	}
	// Another function deployed by this same deployer. Its trigger name
	// has my-func's prefix and carries the shared managed-by annotation.
	other := testKnativeService("my-func-function-trigger-extra")
	colliding := newKnativeTrigger(other, fn.KnativeSubscription{Source: "foreign"}, 0)

	client := &recordingEventingClient{items: []eventingv1.Trigger{*keep, *stale, *similar, *colliding}}
	desired := map[string]*eventingv1.Trigger{keep.Name: keep}

	if err := syncKnativeTriggers(t.Context(), client, ksvc, desired); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	assertTriggerNames(t, client.deleted, stale.Name)
	if len(client.updated) != 0 {
		t.Fatalf("updated %d triggers, want none", len(client.updated))
	}
}

func TestSyncKnativeTriggers_ZeroSubscriptions(t *testing.T) {
	ksvc := testKnativeService("my-func")
	first := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "default"}, 0)
	second := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "other"}, 1)
	similar := &eventingv1.Trigger{
		ObjectMeta: metav1.ObjectMeta{Name: "my-func-function-trigger-backup"},
	}
	other := testKnativeService("my-func-function-trigger-extra")
	colliding := newKnativeTrigger(other, fn.KnativeSubscription{Source: "foreign"}, 0)

	client := &recordingEventingClient{items: []eventingv1.Trigger{*first, *second, *similar, *colliding}}
	if err := syncKnativeTriggers(t.Context(), client, ksvc, map[string]*eventingv1.Trigger{}); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	assertTriggerNames(t, client.deleted, first.Name, second.Name)
	if len(client.updated) != 0 {
		t.Fatalf("updated %d triggers, want none", len(client.updated))
	}
}

func TestSyncKnativeTriggers_ReorderUpdatesWithoutDelete(t *testing.T) {
	ksvc := testKnativeService("my-func")
	first := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "broker-a", Filters: map[string]string{"type": "a"}}, 0)
	second := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "broker-b", Filters: map[string]string{"type": "b"}}, 1)
	desiredFirst := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "broker-b", Filters: map[string]string{"type": "b"}}, 0)
	desiredSecond := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "broker-a", Filters: map[string]string{"type": "a"}}, 1)

	client := &recordingEventingClient{items: []eventingv1.Trigger{*first, *second}}
	desired := map[string]*eventingv1.Trigger{
		desiredFirst.Name:  desiredFirst,
		desiredSecond.Name: desiredSecond,
	}
	if err := syncKnativeTriggers(t.Context(), client, ksvc, desired); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("deleted %v, want none", client.deleted)
	}
	if len(client.updated) != 2 {
		t.Fatalf("updated %d triggers, want 2", len(client.updated))
	}
	got := map[string]string{}
	for _, trigger := range client.updated {
		got[trigger.Name] = trigger.Spec.Broker
	}
	if got[first.Name] != "broker-b" || got[second.Name] != "broker-a" {
		t.Fatalf("reordered brokers = %v", got)
	}
}

func TestSyncKnativeTriggers_EmptyServiceUIDDoesNotDelete(t *testing.T) {
	ksvc := testKnativeService("my-func")
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 1)
	delete(existing.Annotations, knativeTriggerManagedByAnnotation)
	ksvc.UID = ""

	client := &recordingEventingClient{items: []eventingv1.Trigger{*existing}}
	if err := syncKnativeTriggers(t.Context(), client, ksvc, map[string]*eventingv1.Trigger{}); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	if len(client.deleted) != 0 || len(client.updated) != 0 {
		t.Fatalf("empty service UID deleted %v updated %d", client.deleted, len(client.updated))
	}
}

func TestSyncKnativeTriggers_ForeignOwnerSurvives(t *testing.T) {
	ksvc := testKnativeService("my-func")
	existing := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 2)
	existing.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: ksvc.APIVersion,
		Kind:       ksvc.Kind,
		Name:       "someone-else",
		UID:        "other-uid",
	}}

	client := &recordingEventingClient{items: []eventingv1.Trigger{*existing}}
	if err := syncKnativeTriggers(t.Context(), client, ksvc, map[string]*eventingv1.Trigger{}); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	if len(client.deleted) != 0 || len(client.updated) != 0 {
		t.Fatalf("foreign owner deleted %v updated %d", client.deleted, len(client.updated))
	}
}

func TestSyncKnativeTriggers_ForbiddenWithoutSubscriptions(t *testing.T) {
	ksvc := testKnativeService("my-func")
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "eventing.knative.dev", Resource: "triggers"},
		"",
		errors.New("no"),
	)
	client := clienteventingv1.NewMockKnEventingClient(t)
	client.Recorder().ListTriggers(&eventingv1.TriggerList{}, forbidden)

	err := syncKnativeTriggers(t.Context(), client, ksvc, map[string]*eventingv1.Trigger{})
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected forbidden to stay wrapped, got %v", err)
	}
	if got := ignoreTriggerSyncForbidden(err, false, "gitlab-test"); got != nil {
		t.Fatalf("expected no-subscription forbidden to be ignored, got %v", got)
	}
	if got := ignoreTriggerSyncForbidden(err, true, "gitlab-test"); got == nil {
		t.Fatal("expected forbidden to fail when subscriptions are declared")
	}
	client.Recorder().Validate()
}

func TestSyncKnativeTriggers_RecreatedServiceDeletesStale(t *testing.T) {
	ksvc := testKnativeService("my-func")
	stale := newKnativeTrigger(ksvc, fn.KnativeSubscription{Source: "old"}, 1)
	stale.OwnerReferences[0].UID = "previous-service-uid"
	delete(stale.Annotations, knativeTriggerManagedByAnnotation)

	client := &recordingEventingClient{items: []eventingv1.Trigger{*stale}}
	if err := syncKnativeTriggers(t.Context(), client, ksvc, map[string]*eventingv1.Trigger{}); err != nil {
		t.Fatalf("syncKnativeTriggers returned error: %v", err)
	}
	assertTriggerNames(t, client.deleted, stale.Name)
}

func assertTriggerNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("deleted %v, want %v", got, want)
	}
	counts := map[string]int{}
	for _, name := range got {
		counts[name]++
	}
	for _, name := range want {
		if counts[name] != 1 {
			t.Fatalf("deleted %v, want %v", got, want)
		}
	}
}

// recordingEventingClient records trigger deletes and updates. The
// generated mock does not pass the trigger object into UpdateTrigger
// expectations, so reorder tests cannot see the written spec through it.
type recordingEventingClient struct {
	items   []eventingv1.Trigger
	deleted []string
	updated []*eventingv1.Trigger
}

func (c *recordingEventingClient) Namespace() string { return "default" }

func (c *recordingEventingClient) ListTriggers(context.Context) (*eventingv1.TriggerList, error) {
	return &eventingv1.TriggerList{Items: c.items}, nil
}

func (c *recordingEventingClient) DeleteTrigger(_ context.Context, name string) error {
	c.deleted = append(c.deleted, name)
	return nil
}

func (c *recordingEventingClient) UpdateTrigger(_ context.Context, trigger *eventingv1.Trigger) error {
	c.updated = append(c.updated, trigger.DeepCopy())
	return nil
}

func (c *recordingEventingClient) CreateTrigger(context.Context, *eventingv1.Trigger) error {
	return nil
}

func (c *recordingEventingClient) GetTrigger(context.Context, string) (*eventingv1.Trigger, error) {
	return nil, nil
}

func (c *recordingEventingClient) UpdateTriggerWithRetry(context.Context, string, clienteventingv1.TriggerUpdateFunc, int) error {
	return nil
}

func (c *recordingEventingClient) CreateBroker(context.Context, *eventingv1.Broker) error {
	return nil
}

func (c *recordingEventingClient) GetBroker(context.Context, string) (*eventingv1.Broker, error) {
	return nil, nil
}

func (c *recordingEventingClient) DeleteBroker(context.Context, string, time.Duration) error {
	return nil
}

func (c *recordingEventingClient) ListBrokers(context.Context) (*eventingv1.BrokerList, error) {
	return nil, nil
}

func (c *recordingEventingClient) UpdateBroker(context.Context, *eventingv1.Broker) error {
	return nil
}

func (c *recordingEventingClient) UpdateBrokerWithRetry(context.Context, string, clienteventingv1.BrokerUpdateFunc, int) error {
	return nil
}
