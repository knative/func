package operator

import (
	"context"
	"fmt"
	"os"
	"time"

	v1alpha1 "github.com/functions-dev/func-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"knative.dev/func/pkg/k8s"
)

// RegistryCredentials holds resolved credentials for a container registry.
type RegistryCredentials struct {
	Username string
	Password string
	Server   string
}

// SyncConfig holds the parameters for creating/updating a Function CR.
type SyncConfig struct {
	FunctionName string
	Namespace    string
	RepoURL      string
	RepoBranch   string
	RepoPath     string
	// If set, a docker-registry secret is created and referenced from
	// the Function CR's .spec.registry.authSecretRef.
	RegistryCredentials *RegistryCredentials
}

// ensureRegistrySecret creates or updates a docker-registry Secret.
// Defaults to k8s.EnsureDockerRegistrySecretExist; overridden in tests.
var ensureRegistrySecret = k8s.EnsureDockerRegistrySecretExist

// SyncFunctionCR creates or updates a Function CR for the given function.
// It sets up Kubernetes clients, checks if the Function CRD exists on the
// cluster, and creates or updates the CR accordingly.
func SyncFunctionCR(ctx context.Context, cfg SyncConfig) error {
	restCfg, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return fmt.Errorf("getting kubernetes config: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("registering Function scheme: %w", err)
	}

	cl, err := ctrlclient.New(restCfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	return syncFunctionCR(ctx, cl, disc, cfg)
}

func syncFunctionCR(ctx context.Context, cl ctrlclient.Client, disc discovery.DiscoveryInterface, cfg SyncConfig) error {
	hasCRD, err := hasFunctionCRD(disc)
	if err != nil {
		return fmt.Errorf("checking for Function CRD: %w", err)
	}
	if !hasCRD {
		return nil
	}

	if cfg.RepoURL == "" {
		return nil
	}

	// Build desired spec
	var registrySecretRef *v1.LocalObjectReference
	if cfg.RegistryCredentials != nil {
		secretName := cfg.FunctionName + "-registry-auth"
		if err := ensureRegistrySecret(ctx, secretName, cfg.Namespace, nil, nil,
			cfg.RegistryCredentials.Username, cfg.RegistryCredentials.Password, cfg.RegistryCredentials.Server); err != nil {
			return fmt.Errorf("creating registry secret: %w", err)
		}
		registrySecretRef = &v1.LocalObjectReference{Name: secretName}
	}

	desiredSpec := v1alpha1.FunctionSpec{
		Repository: v1alpha1.FunctionSpecRepository{
			URL:    cfg.RepoURL,
			Branch: cfg.RepoBranch,
			Path:   cfg.RepoPath,
		},
	}
	if registrySecretRef != nil {
		desiredSpec.Registry.AuthSecretRef = registrySecretRef
	}

	// Look up existing CR
	existing, err := findExistingCR(ctx, cl, cfg.FunctionName, cfg.Namespace)
	if err != nil {
		return fmt.Errorf("looking up existing Function CR: %w", err)
	}

	if existing != nil {
		existing.Spec = desiredSpec
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		existing.Annotations["functions.knative.dev/last-deployed"] = time.Now().UTC().Format(time.RFC3339)
		if err := cl.Update(ctx, existing); err != nil {
			return fmt.Errorf("updating Function CR: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Function CR %q updated in namespace %q\n", existing.Name, cfg.Namespace)
		return nil
	}

	fn := &v1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.FunctionName,
			Namespace: cfg.Namespace,
			Annotations: map[string]string{
				"functions.knative.dev/last-deployed": time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: desiredSpec,
	}
	if err := cl.Create(ctx, fn); err != nil {
		return fmt.Errorf("creating Function CR: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Function CR %q created in namespace %q\n", cfg.FunctionName, cfg.Namespace)
	return nil
}

func hasFunctionCRD(disc discovery.DiscoveryInterface) (bool, error) {
	resources, err := disc.ServerResourcesForGroupVersion("functions.dev/v1alpha1")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("querying API resources: %w", err)
	}
	for _, r := range resources.APIResources {
		if r.Kind == "Function" {
			return true, nil
		}
	}
	return false, nil
}

func findExistingCR(ctx context.Context, cl ctrlclient.Client, funcName, namespace string) (*v1alpha1.Function, error) {
	var list v1alpha1.FunctionList
	if err := cl.List(ctx, &list, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// Status.Name takes priority over metadata name, so we must scan the
	// full list before falling back to a metadata name match.
	var byName *v1alpha1.Function
	for i := range list.Items {
		if list.Items[i].Status.Name == funcName {
			return &list.Items[i], nil
		}
		if byName == nil && list.Items[i].Name == funcName {
			byName = &list.Items[i]
		}
	}
	return byName, nil
}
