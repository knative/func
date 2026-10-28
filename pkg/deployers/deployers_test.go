package deployers

import "testing"

// TestValidateSwitch covers the deployer-switch policy: the same deployer is
// always allowed and any change of deployer is blocked. The undeployed case
// (any deployer allowed) is the caller's responsibility and is covered by the
// cmd-level deploy tests.
func TestValidateSwitch(t *testing.T) {
	// policy: any re-deployment of a function with different deployer is blocked
	all := []string{Knative, Kubernetes, Keda}

	for _, from := range all {
		for _, to := range all {
			t.Run(from+" to "+to, func(t *testing.T) {
				err := ValidateSwitch(from, to)
				if from == to && err != nil {
					t.Fatalf("expected the same deployer to be a no-op, got: %v", err)
				}
				if from != to && err == nil {
					t.Fatalf("expected %q->%q to be blocked, got nil", from, to)
				}
			})
		}
	}

	// An empty deployer means "not known", not "a deployer named empty":
	// no switch can be established, so none is reported. Guards library
	// callers, which have no CLI to resolve either side for them.
	for _, tt := range []struct {
		name string
		from string
		to   string
	}{
		{"unknown deployed-with is not a switch", "", Keda},
		{"unknown requested is not a switch", Keda, ""},
		{"both unknown is not a switch", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSwitch(tt.from, tt.to); err != nil {
				t.Fatalf("expected %q->%q to be allowed, got: %v", tt.from, tt.to, err)
			}
		})
	}
}

// TestResolve builds a function's deployer up one input at a time: with
// nothing set it is Default, the one it was last deployed with replaces that,
// and the one it asks for replaces both.
func TestResolve(t *testing.T) {
	// Two deployers other than Default, so each step changes the result, also
	// in a build that sets Default to another deployer.
	var others []string
	for _, d := range []string{Knative, Kubernetes, Keda} {
		if d != Default {
			others = append(others, d)
		}
	}
	deployed, intent := others[0], others[1]

	for _, tt := range []struct {
		name     string
		intent   string
		deployed string
		want     string
	}{
		{"nothing set gives Default", "", "", Default},
		{"deployed replaces Default", "", deployed, deployed},
		{"intent replaces deployed", intent, deployed, intent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.intent, tt.deployed); got != tt.want {
				t.Fatalf("Resolve(%q, %q) = %q, want %q", tt.intent, tt.deployed, got, tt.want)
			}
		})
	}
}
