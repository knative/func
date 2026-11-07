package cmd

import (
	"bytes"
	"strings"
	"testing"

	"knative.dev/pkg/ptr"

	fn "knative.dev/func/pkg/functions"
)

// TestWarnScaleKpaIgnore verifies that scale.kpa on a deployer that ignores it
// (raw/keda) produces a warning, while the knative and default deployers -- and
// a function without scale.kpa -- stay silent.
func TestWarnScaleKpaIgnore(t *testing.T) {
	kpa := &fn.ScaleOptions{KPA: &fn.KPAScaleOptions{Metric: ptr.String("concurrency")}}
	minOnly := &fn.ScaleOptions{Min: ptr.Int64(1)}

	tests := []struct {
		name     string
		scale    *fn.ScaleOptions
		deployer string
		warn     bool
	}{
		{"kpa on raw warns", kpa, "raw", true},
		{"kpa on keda warns", kpa, "keda", true},
		{"kpa on knative is silent", kpa, "knative", false},
		{"kpa on default (empty) deployer is silent", kpa, "", false},
		{"no kpa is silent on raw", minOnly, "raw", false},
		{"nil scale is silent", nil, "raw", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			warnScaleKpaIgnore(&buf, tt.scale, tt.deployer)
			warned := strings.Contains(buf.String(), "scale.kpa is ignored")
			if warned != tt.warn {
				t.Errorf("warnScaleKpaIgnore() warned=%v, want %v (output: %q)", warned, tt.warn, buf.String())
			}
		})
	}
}
