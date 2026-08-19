package mcp

import (
	"strings"
	"testing"

	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
)

func TestDeployClientOptions_DeployerSelection(t *testing.T) {
	cfg := ClientConfig{}
	cases := []struct {
		name     string
		deployer string
		wantErr  string
	}{
		{"empty defaults to knative", "", ""},
		{"knative explicit", knative.KnativeDeployerName, ""},
		{"kubernetes", k8s.KubernetesDeployerName, ""},
		{"keda", keda.KedaDeployerName, ""},
		{"unsupported", "lambda", "unsupported deployer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := deployClientOptions("pack", tc.deployer, cfg, false)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(opts) == 0 {
				t.Fatal("expected client options")
			}
		})
	}
}

func TestDerefBoolDefault(t *testing.T) {
	trueVal := true
	falseVal := false
	if !derefBoolDefault(nil, true) {
		t.Fatal("nil should default to true")
	}
	if derefBoolDefault(&trueVal, false) != true {
		t.Fatal("explicit true expected")
	}
	if derefBoolDefault(&falseVal, true) != false {
		t.Fatal("explicit false expected")
	}
}
