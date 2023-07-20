package pac

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/func/pkg/k8s"
)

const (
	infoConfigMap     = "pipelines-as-code-info"
	configMapPacLabel = "app.kubernetes.io/part-of=pipelines-as-code"

	openShiftRouteGroup    = "route.openshift.io"
	openShiftRouteVersion  = "v1"
	openShiftRouteResource = "routes"
	routePacLabel          = "pipelines-as-code/route=controller"
)

// DetectPACInstallation checks whether PAC is installed on the cluster
// Taken and slightly modified from https://github.com/openshift-pipelines/pipelines-as-code/blob/6a7f043f9bb51d04ab729505b26446695595df1f/pkg/cmd/tknpac/bootstrap/bootstrap.go
func DetectPACInstallation(ctx context.Context, wantedNamespace string) (bool, string, error) {
	var installed bool

	clientPac, _, err := NewTektonPacClientAndResolvedNamespace("")
	if err != nil {
		return false, "", err
	}

	clientK8s, _, err := k8s.NewClientAndResolvedNamespace("")
	if err != nil {
		return false, "", err
	}

	_, err = clientPac.Repositories("").List(ctx, metav1.ListOptions{})
	if err != nil && k8serrors.IsNotFound(err) {
		return false, "", nil
	}

	installed = true
	if wantedNamespace != "" {
		_, err := clientK8s.CoreV1().ConfigMaps(wantedNamespace).Get(ctx, infoConfigMap, metav1.GetOptions{})
		if err == nil {
			return installed, wantedNamespace, nil
		}
		return installed, "", fmt.Errorf("could not detect Pipelines as Code configmap in %s namespace : %w, please reinstall", wantedNamespace, err)
	}

	cms, err := clientK8s.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: configMapPacLabel,
	})
	if err == nil {
		for _, cm := range cms.Items {
			if cm.Name == infoConfigMap {
				return installed, cm.Namespace, nil
			}
		}
	}
	return installed, "", fmt.Errorf("could not detect Pipelines as Code configmap on the cluster, please reinstall")
}

// DetectPACOpenShiftRoute detect the openshift route where the pac controller is running
// Taken and slightly modified from https://github.com/openshift-pipelines/pipelines-as-code/blob/0d63e6239f4a7f1fc90decde1e0a154ed56ed0e7/pkg/cmd/tknpac/bootstrap/route.go
func DetectPACOpenShiftRoute(ctx context.Context, targetNamespace string) (string, error) {
	gvr := schema.GroupVersionResource{
		Group: openShiftRouteGroup, Version: openShiftRouteVersion, Resource: openShiftRouteResource,
	}

	client, err := k8s.NewDynamicClient()
	if err != nil {
		return "", err
	}

	routes, err := client.Resource(gvr).Namespace(targetNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: routePacLabel,
	})
	if err != nil {
		return "", err
	}
	if len(routes.Items) != 1 {
		return "", err
	}
	route := routes.Items[0]

	spec, ok := route.Object["spec"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("couldn't find spec in the PAC Controller route")
	}

	host, ok := spec["host"].(string)
	if !ok {
		// this condition is satisfied if there's no metadata at all in the provided CR
		return "", fmt.Errorf("couldn't find spec.host in the PAC controller route")
	}

	return fmt.Sprintf("https://%s", host), nil
}

// GetPACInfo returns the controller url that PAC controller is running
// Taken and slightly modified from https://github.com/openshift-pipelines/pipelines-as-code/blob/0d63e6239f4a7f1fc90decde1e0a154ed56ed0e7/pkg/cli/info/configmap.go
func GetPACInfo(ctx context.Context, namespace string) (string, error) {
	client, namespace, err := k8s.NewClientAndResolvedNamespace(namespace)
	if err != nil {
		return "", err
	}

	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, infoConfigMap, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	return cm.Data["controller-url"], nil
}
