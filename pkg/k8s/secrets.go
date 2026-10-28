package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	k8sclientcmd "k8s.io/client-go/tools/clientcmd"

	fn "knative.dev/func/pkg/functions"
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

// NewRegistrySecret returns the Secret holding the registry credentials of
// function f, of type kubernetes.io/dockerconfigjson, the standard type for
// them, which image pull secrets require. Remote builds mount its
// .dockerconfigjson key as the config.json the build reads. All its writers
// build it here, so none undoes another's write. The function's labels let
// func delete remove it.
func NewRegistrySecret(f fn.Function, username, password, server string) (corev1.Secret, error) {
	labels, err := f.LabelsMap()
	if err != nil {
		return corev1.Secret{}, err
	}
	// go-containerregistry, which the buildpacks build uses, finds a Docker Hub
	// login only under Docker Hub's legacy URL, as docker login writes it.
	if server == name.DefaultRegistry {
		server = authn.DefaultAuthKey
	}
	dockerConfigJSONContent, err := HandleDockerCfgJSONContent(username, password, "", server)
	if err != nil {
		return corev1.Secret{}, err
	}

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name + "-registry-auth",
			Labels:      labels,
			Annotations: f.Deploy.Annotations,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJSONContent},
	}, nil
}

func EnsureSecretExist(ctx context.Context, secret corev1.Secret, namespaceOverride string) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return ensureSecretExist(ctx, client.CoreV1().Secrets(namespace), secret)
}

func ensureSecretExist(ctx context.Context, secrets typedcorev1.SecretInterface, secret corev1.Secret) (err error) {
	// Check whether Secret with specified name exist
	secretNotFound := false
	existingSecret, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return
		}
		secretNotFound = true
	}

	// The API refuses to change a Secret's type, so delete and create anew a
	// registry Secret earlier func versions wrote. Any other Secret may be
	// the user's, so leave it and fail.
	if !secretNotFound && secretType(existingSecret.Type) != secretType(secret.Type) {
		if !isOldRegistrySecret(existingSecret) {
			return fmt.Errorf("secret %q in namespace %q has type %q, but func needs type %q: delete it so func can create it",
				existingSecret.Name, existingSecret.Namespace, secretType(existingSecret.Type), secretType(secret.Type))
		}
		err = secrets.Delete(ctx, secret.Name, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return
		}
		secretNotFound = true
	}

	labelsEqual := true
	annotationsEqual := true
	dataEqual := true

	if !secretNotFound {
		dataEqual = equality.Semantic.DeepDerivative(existingSecret.Data, secret.Data)

		existingLabels := existingSecret.Labels
		if existingLabels == nil {
			existingLabels = map[string]string{}
		}
		desiredLabels := secret.Labels
		if desiredLabels == nil {
			desiredLabels = map[string]string{}
		}
		labelsEqual = equality.Semantic.DeepEqual(existingLabels, desiredLabels)

		existingAnnotations := existingSecret.Annotations
		if existingAnnotations == nil {
			existingAnnotations = map[string]string{}
		}
		desiredAnnotations := secret.Annotations
		if desiredAnnotations == nil {
			desiredAnnotations = map[string]string{}
		}
		annotationsEqual = equality.Semantic.DeepEqual(existingAnnotations, desiredAnnotations)
	}

	if secretNotFound || !dataEqual || !labelsEqual || !annotationsEqual {
		if secretNotFound {
			_, err = secrets.Create(ctx, &secret, metav1.CreateOptions{})
		} else {
			_, err = secrets.Update(ctx, &secret, metav1.UpdateOptions{})
		}
	}

	return
}

// secretType is t as the API stores it: Opaque when empty.
func secretType(t corev1.SecretType) corev1.SecretType {
	if t == "" {
		return corev1.SecretTypeOpaque
	}
	return t
}

// isOldRegistrySecret reports whether s has the shape earlier func versions
// gave the registry Secret: Opaque, with only a config.json key.
func isOldRegistrySecret(s *corev1.Secret) bool {
	_, ok := s.Data["config.json"]
	return ok && len(s.Data) == 1 && secretType(s.Type) == corev1.SecretTypeOpaque
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
