package operator

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/oci"
)

// TestSyncer_RegistrySecret ensures the syncer builds the function's registry
// Secret with k8s.NewRegistrySecret, and only when it finds a login.
func TestSyncer_RegistrySecret(t *testing.T) {
	f := fn.Function{
		Name:    "my-func",
		Runtime: "go",
		Deploy: fn.DeploySpec{
			Image:       "ghcr.io/alice/my-func:latest",
			Annotations: map[string]string{"note": "kept"},
		},
	}
	login := func(username string, err error) oci.CredentialsProvider {
		return func(context.Context, string) (oci.Credentials, error) {
			return oci.Credentials{Username: username, Password: "secret"}, err
		}
	}

	got, err := NewSyncer(WithCredentialsProvider(login("alice", nil))).registrySecret(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	want, err := k8s.NewRegistrySecret(f, "alice", "secret", "ghcr.io")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !equality.Semantic.DeepEqual(*got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for name, s := range map[string]*Syncer{
		"no credentials provider": NewSyncer(),
		"no username":             NewSyncer(WithCredentialsProvider(login("", nil))),
		"no credentials found":    NewSyncer(WithCredentialsProvider(login("alice", errors.New("not found")))),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := s.registrySecret(t.Context(), f)
			if err != nil || got != nil {
				t.Errorf("got %+v, %v; want no Secret", got, err)
			}
		})
	}
}
