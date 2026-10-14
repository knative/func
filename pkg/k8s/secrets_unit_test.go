package k8s

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestEnsureSecretExist ensures ensureSecretExist creates a missing Secret,
// leaves an equal one alone, updates one of the same type in place, and
// replaces one of another type, as the API refuses to change a Secret's type.
// An empty type counts as Opaque.
func TestEnsureSecretExist(t *testing.T) {
	const ns, name = "test-ns", "test-secret"

	secret := func(typ corev1.SecretType, key, value string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Type:       typ,
			Data:       map[string][]byte{key: []byte(value)},
		}
	}

	tests := []struct {
		name     string
		existing *corev1.Secret
		desired  *corev1.Secret
		verbs    []string
	}{
		{
			name:    "created when missing",
			desired: secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:   []string{"get", "create"},
		},
		{
			name:     "replaced when of another type",
			existing: secret(corev1.SecretTypeOpaque, "config.json", "old"),
			desired:  secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:    []string{"get", "delete", "create"},
		},
		{
			name:     "updated in place when of the same type",
			existing: secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "old"),
			desired:  secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:    []string{"get", "update"},
		},
		{
			name:     "kept when equal, an empty type being Opaque",
			existing: secret(corev1.SecretTypeOpaque, "config.json", "same"),
			desired:  secret("", "config.json", "same"),
			verbs:    []string{"get"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			secrets := client.CoreV1().Secrets(ns)
			if tt.existing != nil {
				if _, err := secrets.Create(t.Context(), tt.existing, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			client.ClearActions()

			if err := ensureSecretExist(t.Context(), secrets, *tt.desired); err != nil {
				t.Fatal(err)
			}

			var verbs []string
			for _, a := range client.Actions() {
				verbs = append(verbs, a.GetVerb())
			}
			if !slices.Equal(verbs, tt.verbs) {
				t.Errorf("calls: got %v, want %v", verbs, tt.verbs)
			}

			got, err := secrets.Get(t.Context(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if secretType(got.Type) != secretType(tt.desired.Type) {
				t.Errorf("type: got %q, want %q", got.Type, tt.desired.Type)
			}
			if len(got.Data) != len(tt.desired.Data) {
				t.Errorf("data: got keys %v, want %v", slices.Sorted(maps.Keys(got.Data)), slices.Sorted(maps.Keys(tt.desired.Data)))
			}
			for k, v := range tt.desired.Data {
				if !bytes.Equal(got.Data[k], v) {
					t.Errorf("data[%q]: got %q, want %q", k, got.Data[k], v)
				}
			}
		})
	}
}
