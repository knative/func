package deployers

import "testing"

// TestValidateSwitch covers the deployer-switch policy: the same deployer is
// always allowed, raw -> keda is the one safe cross-switch (keda embeds raw),
// and every other change is blocked. The undeployed case (any deployer allowed)
// is the caller's responsibility and is covered by the cmd-level deploy tests.
func TestValidateSwitch(t *testing.T) {
	for _, tt := range []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"same deployer is a no-op", Keda, Keda, false},
		{"raw to keda is the one safe switch", Kubernetes, Keda, false},
		{"keda to raw is blocked", Keda, Kubernetes, true},
		{"knative to raw is blocked", Knative, Kubernetes, true},
		{"knative to keda is blocked", Knative, Keda, true},
		{"keda to knative is blocked", Keda, Knative, true},
		// An empty deployer means "not known", not "a deployer named empty":
		// no switch can be established, so none is reported. Guards library
		// callers, which have no CLI to resolve either side for them.
		{"unknown deployed-with is not a switch", "", Keda, false},
		{"unknown requested is not a switch", Keda, "", false},
		{"both unknown is not a switch", "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSwitch(tt.from, tt.to)
			if tt.wantErr && err == nil {
				t.Fatalf("expected %q->%q to be blocked, got nil", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected %q->%q to be allowed, got: %v", tt.from, tt.to, err)
			}
		})
	}
}
