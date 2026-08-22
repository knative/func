package deployer

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// Test_GenerateCommonAnnotations_StampNotUserAssignable: the deployer stamp
// decides which component manages an object (Route ownership, describer and
// remover routing), so a same-key annotation in func.yaml must lose to it.
// Deployed objects carry the stamp visibly, and copying `kubectl get -o
// yaml` output into func.yaml's annotations is an ordinary accident.
func Test_IdentityLabels(t *testing.T) {
	got := IdentityLabels("hello", "go")
	if got["boson.dev/function"] != "true" ||
		got["function.knative.dev/name"] != "hello" ||
		got["function.knative.dev/runtime"] != "go" {
		t.Errorf("IdentityLabels = %v", got)
	}
	if len(got) != 3 {
		t.Errorf("IdentityLabels extra keys: %v", got)
	}
}

func Test_SelectorLabels_IdentityOnly(t *testing.T) {
	f := fn.Function{
		Name:    "hello",
		Runtime: "go",
		Domain:  "hello.example.test",
		Deploy: fn.DeploySpec{
			Labels: []fn.Label{{Key: ptrString("team"), Value: ptrString("red")}},
		},
	}
	ll, err := GenerateCommonLabels(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := SelectorLabels(ll)
	want := IdentityLabels("hello", "go")
	if len(got) != len(want) {
		t.Fatalf("SelectorLabels extra keys: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("SelectorLabels[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func ptrString(s string) *string { return &s }

func Test_GenerateCommonAnnotations_StampNotUserAssignable(t *testing.T) {
	f := fn.Function{
		Deploy: fn.DeploySpec{Annotations: map[string]string{
			DeployerNameAnnotation: "banana",
			"my-annotation":        "kept",
		}},
	}

	aa := GenerateCommonAnnotations(f, nil, false, "kubernetes")

	if got := aa[DeployerNameAnnotation]; got != "kubernetes" {
		t.Errorf("stamp = %q, want %q: the ownership stamp must not be user-assignable", got, "kubernetes")
	}
	if got := aa["my-annotation"]; got != "kept" {
		t.Errorf("user annotation = %q, want %q to survive", got, "kept")
	}
}
