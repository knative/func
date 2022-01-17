package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetSecret(ctx context.Context, name, namespaceOverride string) (*corev1.Secret, error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func ListSecretsNames(ctx context.Context, namespaceOverride string) (names []string, err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	secrets, err := client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, s := range secrets.Items {
		names = append(names, s.Name)
	}

	return
}

func DeleteSecrets(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.CoreV1().Secrets(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

func CreateDockerRegistrySecret(ctx context.Context, name, namespaceOverride string, labels map[string]string, username, password, server string) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{},
	}

	dockerConfigJSONContent, err := handleDockerCfgJSONContent(username, password, "", server)
	if err != nil {
		return
	}
	secret.Data[corev1.DockerConfigJsonKey] = dockerConfigJSONContent

	_, err = client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	return
}

// --- Helper methods for DockerConfigJson type of Secret
// Taken from (and converted to private):
// https://github.com/kubernetes/kubectl/blob/10c4667470db41ce138b9aae4e9590dbd7f1930d/pkg/cmd/create/create_secret_docker.go#L290

// DockerConfigJSON represents a local docker auth config file
// for pulling images.
type dockerConfigJSON struct {
	Auths dockerConfig `json:"auths" datapolicy:"token"`
	// +optional
	HttpHeaders map[string]string `json:"HttpHeaders,omitempty" datapolicy:"token"`
}

// DockerConfig represents the config file used by the docker CLI.
// This config that represents the credentials that should be used
// when pulling images from specific image repositories.
type dockerConfig map[string]dockerConfigEntry

// dockerConfigEntry holds the user information that grant the access to docker registry
type dockerConfigEntry struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty" datapolicy:"password"`
	Email    string `json:"email,omitempty"`
	Auth     string `json:"auth,omitempty" datapolicy:"token"`
}

func handleDockerCfgJSONContent(username, password, email, server string) ([]byte, error) {
	dockerConfigAuth := dockerConfigEntry{
		Username: username,
		Password: password,
		Email:    email,
		Auth:     encodeDockerConfigFieldAuth(username, password),
	}
	dockerConfigJSON := dockerConfigJSON{
		Auths: map[string]dockerConfigEntry{server: dockerConfigAuth},
	}

	return json.Marshal(dockerConfigJSON)
}

// encodeDockerConfigFieldAuth returns base64 encoding of the username and password string
func encodeDockerConfigFieldAuth(username, password string) string {
	fieldValue := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(fieldValue))
}
