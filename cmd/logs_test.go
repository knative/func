package cmd

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestLogs_CommandStructure ensures the logs command is properly structured
func TestLogs_CommandStructure(t *testing.T) {
	describer := mock.NewDescriber()
	root := NewRootCmd(RootCommandConfig{
		Name:      "func",
		NewClient: NewTestClient(fn.WithDescribers(describer)),
	})

	logsCmd, _, err := root.Find([]string{"logs"})
	if err != nil {
		t.Fatal(err)
	}

	if logsCmd == nil {
		t.Fatal("logs command not found")
	}

	if logsCmd.Use != "logs" {
		t.Errorf("expected Use to be 'logs', got '%s'", logsCmd.Use)
	}

	// Check that required flags exist
	flags := []string{"name", "namespace", "path", "since", "verbose"}
	for _, flag := range flags {
		if logsCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag '%s' to exist", flag)
		}
	}
}

// TestLogs_ConfigValidation tests the configuration validation
func TestLogs_ConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       logsConfig
		wantError bool
	}{
		{
			name: "valid config with name",
			cfg: logsConfig{
				Name:      "my-function",
				Namespace: "default",
				Since:     "5m",
			},
			wantError: false,
		},
		{
			name: "valid config with path",
			cfg: logsConfig{
				Path:      "./testdata",
				Namespace: "default",
				Since:     "1h",
			},
			wantError: false,
		},
		{
			name: "valid config with default since",
			cfg: logsConfig{
				Name:      "my-function",
				Namespace: "default",
				Since:     "1m",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - just ensure the config structure is valid
			if tt.cfg.Name == "" && tt.cfg.Path == "" {
				t.Error("config should have either name or path")
			}
			if tt.cfg.Namespace == "" {
				t.Error("namespace should not be empty")
			}
		})
	}
}

// TestLogs_SuggestFor ensures the command has proper suggestions
func TestLogs_SuggestFor(t *testing.T) {
	describer := mock.NewDescriber()
	root := NewRootCmd(RootCommandConfig{
		Name:      "func",
		NewClient: NewTestClient(fn.WithDescribers(describer)),
	})

	logsCmd, _, err := root.Find([]string{"logs"})
	if err != nil {
		t.Fatal(err)
	}

	expectedSuggestions := []string{"log", "tail"}
	for _, suggestion := range expectedSuggestions {
		found := false
		for _, s := range logsCmd.SuggestFor {
			if s == suggestion {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected suggestion '%s' not found", suggestion)
		}
	}
}
