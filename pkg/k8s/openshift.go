package k8s

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"

	"knative.dev/func/pkg/docker"
	"knative.dev/func/pkg/docker/creds"
	fn "knative.dev/func/pkg/functions"
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
	var credentials docker.Credentials

	if authInfo := rawConf.AuthInfos[cc.AuthInfo]; authInfo != nil {
		credentials.Username = "openshift"
		credentials.Password = authInfo.Token
	}

	return []creds.CredentialsCallback{
		func(registry string) (docker.Credentials, error) {
			if registry == openShiftRegistryHostPort {
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
		isOpenShift = false
		client, err := NewKubernetesClientset()
		if err != nil {
			return
		}
		_, err = client.CoreV1().Services("openshift-image-registry").Get(context.TODO(), "image-registry", metav1.GetOptions{})
		if err == nil || k8sErrors.IsForbidden(err) {
			isOpenShift = true
			return
		}
	})
	return isOpenShift
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
