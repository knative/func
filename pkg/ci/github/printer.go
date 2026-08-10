package github

import (
	"fmt"
	"io"
)

const (
	MainLayoutPlainText = `
GitHub Workflow Configuration
  Workflow filepath:  %s
  Workflow name:      %s
  Branch:             %s
  Builder:            %s
  Remote build:       %s
  Runner:             %s
  Test step:          %s
  Registry login:     %s
  Manual dispatch:    %s
  Workflow overwrite: %s
`
	RequireManyPlainText = `
  Required Secrets & Variables:
    %s
    %s
    %s
    %s
    %s
`

	RequireOnePlainText = `  Required secret:    %s
`

	PostExportManyPlainText = `
GitHub Workflow created at: %s

Create the following Secrets & Variables on github.com:
  %s
  %s
  %s
  %s
  %s
`

	PostExportOnePlainText = `
GitHub Workflow created at: %s

Create the following Secret on github.com: %s
`
)

func PrintConfiguration(cfg WorkflowConfig, runtime string, w io.Writer) error {
	builder, err := determineBuilder(runtime, cfg.RemoteBuild)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, MainLayoutPlainText,
		cfg.outputPath(),
		cfg.WorkflowName,
		cfg.Branch,
		builder,
		enabledOrDisabled(cfg.RemoteBuild),
		determineRunner(cfg.SelfHostedRunner),
		enabledOrDisabled(cfg.TestStep),
		enabledOrDisabled(cfg.RegistryLogin),
		enabledOrDisabled(cfg.WorkflowDispatch),
		enabledOrDisabled(cfg.Force),
	); err != nil {
		return err
	}

	if cfg.RegistryLogin {
		if _, err := fmt.Fprintf(w, RequireManyPlainText,
			secretsPrefix(cfg.KubeconfigSecret),
			secretsPrefix(cfg.RegistryPassSecret),
			varsPrefix(cfg.RegistryLoginUrlVar),
			varsPrefix(cfg.RegistryUserVar),
			varsPrefix(cfg.RegistryUrlVar),
		); err != nil {
			return err
		}

		return nil
	}

	if _, err := fmt.Fprintf(w,
		RequireOnePlainText,
		secretsPrefix(cfg.KubeconfigSecret),
	); err != nil {
		return err
	}

	return nil
}

func PrintPostExportMessage(opts WorkflowConfig, w io.Writer) error {
	if opts.RegistryLogin {
		_, err := fmt.Fprintf(w, PostExportManyPlainText,
			opts.outputPath(),
			secretsPrefix(opts.KubeconfigSecret),
			secretsPrefix(opts.RegistryPassSecret),
			varsPrefix(opts.RegistryLoginUrlVar),
			varsPrefix(opts.RegistryUserVar),
			varsPrefix(opts.RegistryUrlVar),
		)
		return err
	}

	_, err := fmt.Fprintf(w, PostExportOnePlainText,
		opts.outputPath(),
		secretsPrefix(opts.KubeconfigSecret),
	)
	return err
}

func enabledOrDisabled(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}
