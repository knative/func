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
// leaves an equal one alone, and updates one of the same type in place. As the
// API refuses to change a Secret's type, it replaces one of another type in the
// shape earlier func versions wrote, Opaque with only a config.json key, and
// fails on any other, leaving it as is. An empty type counts as Opaque.
func TestEnsureSecretExist(t *testing.T) {
	const ns, name = "test-ns", "test-secret"

	// secret takes its data as key, value pairs.
	secret := func(typ corev1.SecretType, kv ...string) *corev1.Secret {
		data := map[string][]byte{}
		for i := 0; i < len(kv); i += 2 {
			data[kv[i]] = []byte(kv[i+1])
		}
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Type:       typ,
			Data:       data,
		}
	}

	tests := []struct {
		name     string
		existing *corev1.Secret
		desired  *corev1.Secret
		verbs    []string
		wantErr  bool
	}{
		{
			name:    "created when missing",
			desired: secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:   []string{"get", "create"},
		},
		{
			name:     "replaced when Opaque with only config.json",
			existing: secret(corev1.SecretTypeOpaque, "config.json", "old"),
			desired:  secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:    []string{"get", "delete", "create"},
		},
		{
			name:     "refused when Opaque with more keys",
			existing: secret(corev1.SecretTypeOpaque, "config.json", "old", "provider.token", "token"),
			desired:  secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:    []string{"get"},
			wantErr:  true,
		},
		{
			name:     "refused when of another type than Opaque",
			existing: secret(corev1.SecretTypeBasicAuth, corev1.BasicAuthUsernameKey, "alice"),
			desired:  secret(corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey, "new"),
			verbs:    []string{"get"},
			wantErr:  true,
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

			err := ensureSecretExist(t.Context(), secrets, *tt.desired)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error: got %v, want error %v", err, tt.wantErr)
			}

			var verbs []string
			for _, a := range client.Actions() {
				verbs = append(verbs, a.GetVerb())
			}
			if !slices.Equal(verbs, tt.verbs) {
				t.Errorf("calls: got %v, want %v", verbs, tt.verbs)
			}

			want := tt.desired
			if tt.wantErr {
				want = tt.existing
			}
			got, err := secrets.Get(t.Context(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if secretType(got.Type) != secretType(want.Type) {
				t.Errorf("type: got %q, want %q", got.Type, want.Type)
			}
			if len(got.Data) != len(want.Data) {
				t.Errorf("data: got keys %v, want %v", slices.Sorted(maps.Keys(got.Data)), slices.Sorted(maps.Keys(want.Data)))
			}
			for k, v := range want.Data {
				if !bytes.Equal(got.Data[k], v) {
					t.Errorf("data[%q]: got %q, want %q", k, got.Data[k], v)
				}
			}
		})
	}
}
