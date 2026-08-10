package github

import (
	"context"
	"fmt"
	"io"
	"os"

	fn "knative.dev/func/pkg/functions"
)

type workflowGenerator struct {
	verbose        bool
	workflowWriter WorkflowWriter
	messageWriter  io.Writer
	cfg            WorkflowConfig
}

// Option configures a workflowGenerator.
type Option func(*workflowGenerator)

// WithWorkflowConfig overrides the default workflow configuration.
// Empty string fields are backfilled with defaults after all options
// are applied. Boolean fields are used as-is since their zero value
// (false) is indistinguishable from an explicit false.
func WithWorkflowConfig(wc WorkflowConfig) Option {
	return func(wg *workflowGenerator) {
		wg.cfg = wc
	}
}

// WithVerbose enables detailed configuration output after generation.
func WithVerbose(v bool) Option {
	return func(wg *workflowGenerator) {
		wg.verbose = v
	}
}

// WithWorkflowWriter sets the writer used to persist the workflow file.
func WithWorkflowWriter(ww WorkflowWriter) Option {
	return func(wg *workflowGenerator) {
		wg.workflowWriter = ww
	}
}

// WithMessageWriter sets the writer used for user-facing messages.
func WithMessageWriter(mw io.Writer) Option {
	return func(wg *workflowGenerator) {
		wg.messageWriter = mw
	}
}

// NewWorkflowGenerator creates a workflow generator with sensible
// defaults: writes to disk via DefaultWorkflowWriter, prints to
// os.Stdout, and uses a full default WorkflowConfig. All defaults
// can be overridden via options. If WithWorkflowConfig is used,
// the provided config replaces the defaults and any empty string
// fields are backfilled with defaults.
func NewWorkflowGenerator(options ...Option) *workflowGenerator {
	wg := &workflowGenerator{
		cfg:            defaultWorkflowConfig(),
		workflowWriter: DefaultWorkflowWriter,
		messageWriter:  os.Stdout,
	}

	for _, o := range options {
		o(wg)
	}

	wg.cfg = setEmptyFieldsToDefaults(wg.cfg)

	return wg
}

// Generate creates a GitHub Actions workflow file for the given function.
func (g *workflowGenerator) Generate(ctx context.Context, f fn.Function) error {
	if f.Root == "" {
		return fmt.Errorf("function root path can not be empty")
	}

	githubWorkflow, err := newGitHubWorkflow(g.cfg, f.Runtime, g.messageWriter)
	if err != nil {
		return err
	}

	if err := githubWorkflow.Export(g.cfg.fnGitHubWorkflowFilepath(f.Root), g.workflowWriter, g.cfg.Force, g.messageWriter); err != nil {
		return err
	}

	if g.verbose {
		// best-effort user message; errors are non-critical
		_ = PrintConfiguration(g.cfg, f.Runtime, g.messageWriter)
		return nil
	}

	// best-effort user message; errors are non-critical
	_ = PrintPostExportMessage(g.cfg, g.messageWriter)
	return nil
}

// WorkflowGeneratorMock is a test double that records calls to Generate
// for assertion in tests.
type WorkflowGeneratorMock struct {
	WasInvoked bool           // true after Generate is called
	Config     WorkflowConfig // captured config from factory
	FnRoot     string         // captured function root from Generate
}

// Generate implements fn.CIGenerator by recording the call.
func (gm *WorkflowGeneratorMock) Generate(_ context.Context, f fn.Function) error {
	gm.WasInvoked = true
	gm.FnRoot = f.Root

	return nil
}
