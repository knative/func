package github

import (
	"context"
	"fmt"
	"io"

	fn "knative.dev/func/pkg/functions"
)

const DefaultVerbose = false

type workflowGenerator struct {
	verbose bool
}

func NewWorkflowGenerator(verbose bool) *workflowGenerator {
	return &workflowGenerator{verbose}
}

func (g *workflowGenerator) Generate(ctx context.Context, config any, pathWriter fn.PathWriter, messageWriter io.Writer) error {
	cfg, ok := config.(Config)
	if !ok {
		return fmt.Errorf("incorrect type of Config: %T", config)
	}

	githubWorkflow, err := newGitHubWorkflow(cfg, messageWriter)
	if err != nil {
		return err
	}

	if err := githubWorkflow.Export(cfg.FnGitHubWorkflowFilepath(), pathWriter, cfg.Force, messageWriter); err != nil {
		return err
	}

	if g.verbose {
		// best-effort user message; errors are non-critical
		_ = PrintConfiguration(messageWriter, cfg)
		return nil
	}

	// best-effort user message; errors are non-critical
	_ = PrintPostExportMessage(messageWriter, cfg)
	return nil
}
