package k8s

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
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

// IsOpenshift detects whether the target cluster is OpenShift by checking
// for the route.openshift.io API group. The result is cached per Client instance.
func (c *Client) IsOpenshift() bool {
	c.openShiftOnce.Do(func() {
		clientset, err := c.Clientset()
		if err != nil {
			return
		}
		_, err = clientset.Discovery().ServerResourcesForGroupVersion("route.openshift.io/v1")
		// if this group version is found == Openshift cluster
		c.openshift = err == nil
	})
	return c.openshift
}

func (c *Client) GetOpenShiftServiceCA(ctx context.Context) (*x509.Certificate, error) {
	client, ns, err := c.ClientAndNamespace("")
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

func GetDefaultOpenShiftRegistry(c *Client) string {
	ns, _ := c.DefaultNamespace()
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

func GetOpenShiftDockerCredentialLoaders(c *Client) []creds.CredentialsCallback {
	restCfg, err := c.ClientConfig()
	if err != nil || restCfg.BearerToken == "" {
		return nil
	}

	credentials := oci.Credentials{
		Username: "openshift",
		Password: restCfg.BearerToken,
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
