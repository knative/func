package knative

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func TestValidateKubeconfigFile(t *testing.T) {
	testDir := t.TempDir()
	existingPath := filepath.Join(testDir, "kubeconfig")
	if err := os.WriteFile(existingPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	missingPath := filepath.Join(testDir, "missing")
	listSep := string(filepath.ListSeparator)

	cases := []struct {
		name        string
		kubeconfig  string
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "empty env",
			kubeconfig: "",
			wantErr:    false,
		},
		{
			name:       "single existing",
			kubeconfig: existingPath,
			wantErr:    false,
		},
		{
			name:        "single missing",
			kubeconfig:  missingPath,
			wantErr:     true,
			wantErrText: "kubeconfig file does not exist at path",
		},
		{
			name:       "multiple with existing",
			kubeconfig: missingPath + listSep + existingPath,
			wantErr:    false,
		},
		{
			name:        "multiple all missing",
			kubeconfig:  missingPath + listSep + filepath.Join(testDir, "missing2"),
			wantErr:     true,
			wantErrText: "kubeconfig file does not exist at path",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KUBECONFIG", tc.kubeconfig)

			err := validateKubeconfigFile()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !errors.Is(err, fn.ErrInvalidKubeconfig) {
					t.Fatalf("expected ErrInvalidKubeconfig, got: %v", err)
				}
				if tc.wantErrText != "" && !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("expected error to contain %q, got: %v", tc.wantErrText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
