package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclientcmd "k8s.io/client-go/tools/clientcmd"
)

func GetSecret(ctx context.Context, name, namespaceOverride string) (*corev1.Secret, error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListSecretsNamesIfConnected lists names of Secrets present and the current k8s context
// returns empty list, if not connected to any cluster
func ListSecretsNamesIfConnected(ctx context.Context, namespaceOverride string) (names []string, err error) {
	names, err = listSecretsNames(ctx, namespaceOverride)
	if err != nil {
		// not logged our authorized to access resources
		if k8serrors.IsForbidden(err) || k8serrors.IsUnauthorized(err) || k8serrors.IsInvalid(err) || k8serrors.IsTimeout(err) {
			return []string{}, nil
		}

		// non existent k8s cluster
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			if dnsErr.IsNotFound || dnsErr.IsTemporary || dnsErr.IsTimeout {
				return []string{}, nil
			}
		}

		// connection refused
		if errors.Is(err, syscall.ECONNREFUSED) {
			return []string{}, nil
		}

		// invalid configuration: no configuration has been provided
		if k8sclientcmd.IsEmptyConfig(err) {
			return []string{}, nil
		}
	}

	return
}

func listSecretsNames(ctx context.Context, namespaceOverride string) (names []string, err error) {
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

func EnsureDockerRegistrySecretExist(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, username, password, server string) (err error) {
	dockerConfigJSONContent, err := HandleDockerCfgJSONContent(username, password, "", server)
	if err != nil {
		return
	}

	// Check whether we need to create or update the Secret
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	secret.Data["config.json"] = dockerConfigJSONContent

	return EnsureSecretExist(ctx, secret, namespaceOverride)
}

func EnsureSecretExist(ctx context.Context, secret corev1.Secret, namespaceOverride string) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	// Check whether Secret with specified name exist
	secretNotFound := false
	existingSecret, err := GetSecret(ctx, secret.Name, namespace)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return
		}
		secretNotFound = true
	}

	// TODO we should also compare labels and annotations
	if secretNotFound || !equality.Semantic.DeepDerivative(existingSecret.Data, secret.Data) {
		// Decide whether create or update
		if secretNotFound {
			_, err = client.CoreV1().Secrets(namespace).Create(ctx, &secret, metav1.CreateOptions{})
		} else {
			_, err = client.CoreV1().Secrets(namespace).Update(ctx, &secret, metav1.UpdateOptions{})
		}
	}

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

func HandleDockerCfgJSONContent(username, password, email, server string) ([]byte, error) {
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
