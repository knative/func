package tekton

import (
	"context"
	"fmt"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/pipelines"
	"knative.dev/func/pkg/pipelines/tekton/pac"
)

// ensurePACSecretExists checks that up-to-date secret holding credentials needed for PAC is on the cluster
func ensurePACSecretExists(ctx context.Context, f fn.Function, namespace string, credentials pipelines.PacMetadata, labels map[string]string) error {
	dockerConfigJSONContent, err := k8s.HandleDockerCfgJSONContent(credentials.RegistryUsername, credentials.RegistryPassword, "", credentials.RegistryServer)
	if err != nil {
		return err
	}

	// Check whether we need to create or update the Secret
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getPipelineSecretName(f),
			Labels:      labels,
			Annotations: f.Deploy.Annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	secret.Data["config.json"] = dockerConfigJSONContent
	secret.Data["provider.token"] = []byte(credentials.PersonalAccessToken)
	secret.Data["webhook.secret"] = []byte(credentials.WebhookSecret)

	return k8s.EnsureSecretExist(ctx, secret, namespace)
}

// ensurePACRepositoryExists checks that up-to-date Repository CR is present on the cluster
func ensurePACRepositoryExists(ctx context.Context, f fn.Function, namespace string, metadata pipelines.PacMetadata, labels map[string]string) error {
	client, namespace, err := pac.NewTektonPacClientAndResolvedNamespace(namespace)
	if err != nil {
		return err
	}

	repoName := getPipelineRepositoryName(f)
	repo := v1alpha1.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:        repoName,
			Labels:      labels,
			Annotations: f.Deploy.Annotations,
		},
		Spec: v1alpha1.RepositorySpec{
			URL: f.Build.Git.URL,
			GitProvider: &v1alpha1.GitProvider{
				Type: metadata.GitProvider,
				Secret: &v1alpha1.Secret{
					Name: getPipelineSecretName(f),
				},
				WebhookSecret: &v1alpha1.Secret{
					Name: getPipelineSecretName(f),
				},
			},
		},
	}

	repoNotFound := false
	existingRepo, err := client.Repositories(namespace).Get(ctx, repoName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		repoNotFound = true
	}

	// TODO we should also compare labels and annotations
	if repoNotFound || !equality.Semantic.DeepDerivative(existingRepo.Spec, repo.Spec) {
		// Decide whether create or update
		if repoNotFound {
			_, err = client.Repositories(namespace).Create(ctx, &repo, metav1.CreateOptions{})
		} else {
			_, err = client.Repositories(namespace).Update(ctx, &repo, metav1.UpdateOptions{})
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// deletePACRepositories deletes all Repository resources present on the cluster that match input list options
func deletePACRepositories(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) error {
	client, namespace, err := pac.NewTektonPacClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return err
	}

	return client.Repositories(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

// getPipelineRepositoryName generates name for Repository CR
func getPipelineRepositoryName(f fn.Function) string {
	return fmt.Sprintf("%s-repo", getPipelineName(f))
}
