package k8s

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"

	"knative.dev/func/pkg/creds"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
)

const (
	openShiftRegistryHost     = "image-registry.openshift-image-registry.svc"
	openShiftRegistryHostPort = openShiftRegistryHost + ":5000"
)

func GetOpenShiftServiceCA(ctx context.Context) (*x509.Certificate, error) {
	client, ns, err := NewClientAndResolvedNamespace("")
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

func GetDefaultOpenShiftRegistry() string {
	ns, _ := GetDefaultNamespace()
	if ns == "" {
		ns = "default"
	}

	return openShiftRegistryHostPort + "/" + ns
}

// IsOpenShiftInternalRegistry returns true if the given registry string
// refers to the OpenShift internal image registry.
func IsOpenShiftInternalRegistry(registry string) bool {
	return strings.HasPrefix(registry, openShiftRegistryHost)
}

func GetOpenShiftDockerCredentialLoaders() []creds.CredentialsCallback {
	conf := GetClientConfig()

	rawConf, err := conf.RawConfig()
	if err != nil {
		return nil
	}

	cc, ok := rawConf.Contexts[rawConf.CurrentContext]
	if !ok {
		return nil
	}
	var credentials oci.Credentials

	if authInfo := rawConf.AuthInfos[cc.AuthInfo]; authInfo != nil {
		credentials.Username = "openshift"
		credentials.Password = authInfo.Token
	}

	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if registry == openShiftRegistryHostPort {
				return credentials, nil
			}
			return oci.Credentials{}, creds.ErrCredentialsNotFound
		},
	}

}

// openShiftRouteGroupVersion is the API group whose presence identifies an
// OpenShift cluster. Routes are OpenShift-specific, and discovery should answer
// it even under restrictive RBAC, unlike listing namespaces or services.
const openShiftRouteGroupVersion = "route.openshift.io/v1"

var (
	detectOnce  sync.Once
	isOpenShift bool
	detectErr   error
)

// DetectOpenShift reports whether the cluster serves the OpenShift Route API.
// A non-nil error means the cluster could not be asked and the bool is
// meaningless. Probes once per process, answers from cache after.
func DetectOpenShift() (bool, error) {
	detectOnce.Do(func() {
		client, err := NewKubernetesClientset()
		if err != nil {
			detectErr = err
			return
		}
		_, err = client.Discovery().ServerResourcesForGroupVersion(openShiftRouteGroupVersion)
		switch {
		case err == nil:
			isOpenShift = true
		case apierrors.IsNotFound(err):
			// The cluster answered: it does not serve this API.
		default:
			detectErr = err
		}
	})
	return isOpenShift, detectErr
}

// IsOpenShift is a convenient wrapper for getting simple yes/no for openshift
// cluster. The inner function should run in the cmd layer once to resolve the
// detectOnce.Do(), any call after is cached so we dont have to call API all the
// time.
//
// note: gauron99: this might change after restructuring to kubeconfig resolution
// at the start of program instead of adhoc API calls of kube client throughout
// the codebase
func IsOpenShift() bool {
	ok, _ := DetectOpenShift()
	return ok
}

// SetOpenShiftForTest seeds the detection cache; err simulates a cluster that
// could not be asked. Returns a cleanup restoring the previous state.
func SetOpenShiftForTest(val bool, err error) func() {
	detectOnce.Do(func() {}) // ensure real detection won't run
	prevB, prevE := isOpenShift, detectErr
	isOpenShift, detectErr = val, err
	return func() { isOpenShift, detectErr = prevB, prevE }
}

const (
	annotationOpenShiftVcsUri = "app.openshift.io/vcs-uri"
	annotationOpenShiftVcsRef = "app.openshift.io/vcs-ref"

	labelAppK8sInstance   = "app.kubernetes.io/instance"
	labelOpenShiftRuntime = "app.openshift.io/runtime"
)

var iconValuesForRuntimes = map[string]string{
	"go":         "golang",
	"node":       "nodejs",
	"python":     "python",
	"quarkus":    "quarkus",
	"springboot": "spring-boot",
}

type OpenshiftMetadataDecorator struct{}

func (o OpenshiftMetadataDecorator) UpdateAnnotations(f fn.Function, annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationOpenShiftVcsUri] = f.Build.Git.URL
	annotations[annotationOpenShiftVcsRef] = f.Build.Git.Revision

	return annotations
}

func (o OpenshiftMetadataDecorator) UpdateLabels(f fn.Function, labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}

	// this label is used for referencing a Tekton Pipeline and deployed KService
	labels[labelAppK8sInstance] = f.Name

	// if supported, set the label representing a runtime icon in Developer Console
	iconValue, ok := iconValuesForRuntimes[f.Runtime]
	if ok {
		labels[labelOpenShiftRuntime] = iconValue
	}

	return labels
}
