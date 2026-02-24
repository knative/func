package functions_test

import (
	"errors"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func TestFormatList(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{
			name:  "empty list",
			items: []string{},
			want:  "",
		},
		{
			name:  "single item",
			items: []string{"wasm"},
			want:  "wasm",
		},
		{
			name:  "two items",
			items: []string{"pack", "s2i"},
			want:  "pack and s2i",
		},
		{
			name:  "three items",
			items: []string{"pack", "s2i", "host"},
			want:  "pack, s2i, and host",
		},
		{
			name:  "four items",
			items: []string{"knative", "raw", "keda", "wasm"},
			want:  "knative, raw, keda, and wasm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fn.FormatList(tt.items)
			if got != tt.want {
				t.Errorf("FormatList(%v) = %q, want %q", tt.items, got, tt.want)
			}
		})
	}
}

func TestErrIncompatibleBuilder(t *testing.T) {
	tests := []struct {
		name    string
		err     *fn.ErrIncompatibleBuilder
		wantMsg string
	}{
		{
			name: "wasm builder with traditional runtime",
			err: &fn.ErrIncompatibleBuilder{
				Runtime:       "go",
				Builder:       "wasm",
				ValidBuilders: []string{"pack", "s2i", "host"},
			},
			wantMsg: `builder "wasm" is not compatible with runtime "go". Valid builders for this runtime are: pack, s2i, and host`,
		},
		{
			name: "pack builder with WASI runtime",
			err: &fn.ErrIncompatibleBuilder{
				Runtime:       "rust-wasi",
				Builder:       "pack",
				ValidBuilders: []string{"wasm"},
			},
			wantMsg: `builder "pack" is not compatible with runtime "rust-wasi". Valid builders for this runtime are: wasm`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("ErrIncompatibleBuilder.Error() = %q, want %q", got, tt.wantMsg)
			}
			if !errors.Is(tt.err, fn.ErrIncompatibility) {
				t.Errorf("ErrIncompatibleBuilder should unwrap to ErrIncompatibility")
			}
		})
	}
}

func TestErrIncompatibleDeployer(t *testing.T) {
	tests := []struct {
		name    string
		err     *fn.ErrIncompatibleDeployer
		wantMsg string
	}{
		{
			name: "wasm deployer with traditional runtime",
			err: &fn.ErrIncompatibleDeployer{
				Runtime:        "node",
				Deployer:       "wasm",
				ValidDeployers: []string{"knative", "raw", "keda"},
			},
			wantMsg: `deployer "wasm" is not compatible with runtime "node". Valid deployers for this runtime are: knative, raw, and keda`,
		},
		{
			name: "knative deployer with WASI runtime",
			err: &fn.ErrIncompatibleDeployer{
				Runtime:        "go-wasi",
				Deployer:       "knative",
				ValidDeployers: []string{"wasm"},
			},
			wantMsg: `deployer "knative" is not compatible with runtime "go-wasi". Valid deployers for this runtime are: wasm`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("ErrIncompatibleDeployer.Error() = %q, want %q", got, tt.wantMsg)
			}
			if !errors.Is(tt.err, fn.ErrIncompatibility) {
				t.Errorf("ErrIncompatibleDeployer should unwrap to ErrIncompatibility")
			}
		})
	}
}

func TestErrIncompatibleConfiguration(t *testing.T) {
	err := fn.ErrIncompatibleConfiguration{
		Runtime:  "go",
		Builder:  "wasm",
		Deployer: "knative",
		Reason:   "wasm builder does not support traditional runtimes",
	}
	want := `incompatible configuration: runtime="go", builder="wasm", deployer="knative": wasm builder does not support traditional runtimes`
	if got := err.Error(); got != want {
		t.Errorf("ErrIncompatibleConfiguration.Error() = %q, want %q", got, want)
	}
}
