//go:build integration

package k8s_test

import (
	"bytes"
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/k8s"
)

// TestInt_EnsureDockerRegistrySecretExist ensures it replaces the registry
// Secret earlier func versions wrote, Opaque with only a config.json key, as
// the API refuses to change a Secret's type, and then updates it in place.
func TestInt_EnsureDockerRegistrySecretExist(t *testing.T) {
	cliSet, ns, err := k8s.NewClientAndResolvedNamespace("")
	if err != nil {
		t.Fatal(err)
	}
	secrets := cliSet.CoreV1().Secrets(ns)
	name := "testing-registry-secret-" + rand.String(5)

	_, err = secrets.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"config.json": []byte(`{"auths":{}}`)},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = secrets.Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	const server, username = "registry.example.com", "alice"
	for _, password := range []string{"first", "second"} {
		if err = k8s.EnsureDockerRegistrySecretExist(t.Context(), name, ns, nil, nil, username, password, server); err != nil {
			t.Fatal(err)
		}
		s, err := secrets.Get(t.Context(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Type != corev1.SecretTypeDockerConfigJson {
			t.Errorf("type: got %q, want %q", s.Type, corev1.SecretTypeDockerConfigJson)
		}
		if _, ok := s.Data["config.json"]; ok {
			t.Error("the Opaque Secret's config.json key remains")
		}
		want, err := k8s.HandleDockerCfgJSONContent(username, password, "", server)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(s.Data[corev1.DockerConfigJsonKey], want) {
			t.Errorf("%s: got %s, want %s", corev1.DockerConfigJsonKey, s.Data[corev1.DockerConfigJsonKey], want)
		}
	}
}
