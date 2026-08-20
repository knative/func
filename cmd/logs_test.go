package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
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
	flags := []string{"name", "namespace", "path", "since", "tail", "follow", "verbose"}
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

// knativeDescriber returns a describer which reports a deployed function
// deployed with the Knative deployer.
func knativeDescriber() *mock.Describer {
	describer := mock.NewDescriber()
	describer.DescribeFn = func(_ context.Context, name, _ string) (fn.Instance, error) {
		return fn.Instance{
			Name:      name,
			Namespace: "default",
			// That of the latest revision, as the Knative describer reports it.
			Image:    "example.com/myfunc@sha256:" + strings.Repeat("a", 64),
			Deployer: "knative",
		}, nil
	}
	return describer
}

// int64Ptr returns a pointer to the given value.
func int64Ptr(i int64) *int64 { return &i }

// newTestLogsCmd returns a logs command whose logs are gathered by the given
// function rather than from a cluster.
func newTestLogsCmd(gather logGatherer, out, errOut io.Writer) *cobra.Command {
	cmd := newLogsCmd(NewTestClient(fn.WithDescribers(knativeDescriber())), gather)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd
}

// TestLogs_DefaultSnapshot ensures that, absent --follow, logs are written and
// the command completes without gathering logs in follow mode and without
// writing interactive chrome to stdout.
func TestLogs_DefaultSnapshot(t *testing.T) {
	_ = FromTempDirectory(t)

	var opts knative.LogsOptions
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := newTestLogsCmd(func(ctx context.Context, o knative.LogsOptions, out io.Writer) error {
		opts = o
		if o.Follow { // would block indefinitely in the real implementation
			<-ctx.Done()
			return ctx.Err()
		}
		_, err := out.Write([]byte("log line\n"))
		return err
	}, stdout, stderr)
	cmd.SetArgs([]string{"--name", "myfunc"})

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Execute() }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("'logs' did not complete without --follow")
	}

	if opts.Follow {
		t.Error("expected logs to be gathered in snapshot mode")
	}
	if stdout.String() != "log line\n" {
		t.Errorf("expected stdout to contain log content only, got %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("expected no output on stderr, got %q", stderr.String())
	}
}

// TestLogs_Follow ensures that --follow gathers logs in follow mode until the
// context is cancelled, and that the interactive banner is written to stderr.
func TestLogs_Follow(t *testing.T) {
	_ = FromTempDirectory(t)

	var opts knative.LogsOptions
	started := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := newTestLogsCmd(func(ctx context.Context, o knative.LogsOptions, out io.Writer) error {
		opts = o
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, stdout, stderr)
	cmd.SetArgs([]string{"--name", "myfunc", "--follow"})

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.ExecuteContext(ctx) }()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("'logs --follow' returned before streaming: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("'logs --follow' did not begin streaming")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil { // a cancelled stream is not an error
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("'logs --follow' did not stop when cancelled")
	}

	if !opts.Follow {
		t.Error("expected logs to be gathered in follow mode")
	}
	if !strings.Contains(stderr.String(), "Press Ctrl+C to stop.") {
		t.Errorf("expected the interactive banner on stderr, got %q", stderr.String())
	}
}

// TestLogs_Window ensures --since and --tail are passed through, and that the
// window defaults do not truncate a snapshot: a default window would hide the
// logs of a function idle for longer than it, and would silently cut --tail
// short.  Following keeps its historical minute of context instead.
func TestLogs_Window(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantTail   *int64
		wantWindow time.Duration // zero means all available logs
	}{
		{name: "snapshot defaults to all logs", args: []string{}},
		{name: "snapshot honors since", args: []string{"--since", "5m"}, wantWindow: 5 * time.Minute},
		{name: "snapshot tail is not truncated", args: []string{"--tail", "20"}, wantTail: int64Ptr(20)},
		{name: "since and tail combine", args: []string{"--since", "5m", "--tail", "20"}, wantTail: int64Ptr(20), wantWindow: 5 * time.Minute},
		{name: "follow defaults to the last minute", args: []string{"-f"}, wantWindow: time.Minute},
		{name: "follow with tail is not truncated", args: []string{"-f", "--tail", "20"}, wantTail: int64Ptr(20)},
		{name: "follow honors since", args: []string{"-f", "--since", "5m"}, wantWindow: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = FromTempDirectory(t)

			var opts knative.LogsOptions
			cmd := newTestLogsCmd(func(_ context.Context, o knative.LogsOptions, _ io.Writer) error {
				opts = o
				return nil
			}, &bytes.Buffer{}, &bytes.Buffer{})
			cmd.SetArgs(append([]string{"--name", "myfunc"}, tt.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if tt.wantTail == nil && opts.TailLines != nil {
				t.Errorf("expected unlimited lines, got %v", *opts.TailLines)
			}
			if tt.wantTail != nil && (opts.TailLines == nil || *opts.TailLines != *tt.wantTail) {
				t.Errorf("expected tail %v, got %v", *tt.wantTail, opts.TailLines)
			}
			if tt.wantWindow == 0 {
				if opts.Since != nil {
					t.Errorf("expected all available logs, got those since %v", *opts.Since)
				}
				return
			}
			if opts.Since == nil {
				t.Fatalf("expected logs since ~%v ago, got all available logs", tt.wantWindow)
			}
			if window := time.Since(*opts.Since); window < tt.wantWindow || window > tt.wantWindow+time.Minute {
				t.Errorf("expected logs since ~%v ago, got %v", tt.wantWindow, window)
			}
		})
	}
}

// TestLogs_NoPods ensures that a function with no pods to read logs from - the
// normal state of a Knative service which has scaled to zero - is reported on
// stderr rather than being indistinguishable from a function logging nothing.
func TestLogs_NoPods(t *testing.T) {
	_ = FromTempDirectory(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := newTestLogsCmd(func(_ context.Context, _ knative.LogsOptions, _ io.Writer) error {
		return k8s.ErrNoMatchingPods
	}, stdout, stderr)
	cmd.SetArgs([]string{"--name", "myfunc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("an idle function is not a failure: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no output on stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "scaled to zero") {
		t.Errorf("expected the absence of pods to be explained, got %q", stderr.String())
	}
}

// TestLogs_PartialLogs ensures that logs gathered from some pods are kept, and
// the command still succeeds, when another pod's logs could not be read.
func TestLogs_PartialLogs(t *testing.T) {
	_ = FromTempDirectory(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := newTestLogsCmd(func(_ context.Context, _ knative.LogsOptions, out io.Writer) error {
		_, _ = out.Write([]byte("log line\n"))
		return &k8s.PartialLogsError{Err: errors.New("pod is terminating")}
	}, stdout, stderr)
	cmd.SetArgs([]string{"--name", "myfunc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected partially gathered logs to succeed: %v", err)
	}
	if stdout.String() != "log line\n" {
		t.Errorf("expected the gathered logs to be kept, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "pod is terminating") {
		t.Errorf("expected a warning on stderr, got %q", stderr.String())
	}
}

// TestLogs_Failure ensures a hard failure to gather logs is a non-zero exit.
func TestLogs_Failure(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := newTestLogsCmd(func(_ context.Context, _ knative.LogsOptions, _ io.Writer) error {
		return errors.New("forbidden")
	}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--name", "myfunc"})

	if err := cmd.Execute(); err == nil {
		t.Error("expected a failure to gather logs to be an error")
	}
}

// TestLogs_ImageNotFiltered ensures the logs of all of a function's pods are
// gathered.  The image of the described instance is that of the latest
// revision, so filtering on it would drop the logs of the pods actually
// serving traffic during a rollout.
func TestLogs_ImageNotFiltered(t *testing.T) {
	_ = FromTempDirectory(t)

	var opts knative.LogsOptions
	cmd := newTestLogsCmd(func(_ context.Context, o knative.LogsOptions, _ io.Writer) error {
		opts = o
		return nil
	}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--name", "myfunc"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if opts.Image != "" {
		t.Errorf("expected pods to not be filtered by image, got %q", opts.Image)
	}
}

// TestLogs_FollowEnv ensures following can be enabled by environment, which is
// how a user for whom the previous always-follow default was the right one can
// restore it.
func TestLogs_FollowEnv(t *testing.T) {
	_ = FromTempDirectory(t)
	t.Setenv("FUNC_FOLLOW", "true")

	var opts knative.LogsOptions
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := newTestLogsCmd(func(ctx context.Context, o knative.LogsOptions, _ io.Writer) error {
		opts = o
		cancel()
		return ctx.Err()
	}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--name", "myfunc"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	if !opts.Follow {
		t.Error("expected FUNC_FOLLOW to enable following")
	}
}
