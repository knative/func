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
	Remote build        %s
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

func PrintConfiguration(w io.Writer, conf Config) error {
	builder, err := determineBuilder(conf.FnRuntime, conf.RemoteBuild)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, MainLayoutPlainText,
		conf.OutputPath(),
		conf.WorkflowName,
		conf.Branch,
		builder,
		enabledOrDisabled(conf.RemoteBuild),
		determineRunner(conf.SelfHostedRunner),
		enabledOrDisabled(conf.TestStep),
		enabledOrDisabled(conf.RegistryLogin),
		enabledOrDisabled(conf.WorkflowDispatch),
		enabledOrDisabled(conf.Force),
	); err != nil {
		return err
	}

	if conf.RegistryLogin {
		if _, err := fmt.Fprintf(w, RequireManyPlainText,
			secretsPrefix(conf.KubeconfigSecret),
			secretsPrefix(conf.RegistryPassSecret),
			varsPrefix(conf.RegistryLoginUrlVar),
			varsPrefix(conf.RegistryUserVar),
			varsPrefix(conf.RegistryUrlVar),
		); err != nil {
			return err
		}

		return nil
	}

	if _, err := fmt.Fprintf(w,
		RequireOnePlainText,
		secretsPrefix(conf.KubeconfigSecret),
	); err != nil {
		return err
	}

	return nil
}

func PrintPostExportMessage(w io.Writer, conf Config) error {
	if conf.RegistryLogin {
		_, err := fmt.Fprintf(w, PostExportManyPlainText,
			conf.OutputPath(),
			secretsPrefix(conf.KubeconfigSecret),
			secretsPrefix(conf.RegistryPassSecret),
			varsPrefix(conf.RegistryLoginUrlVar),
			varsPrefix(conf.RegistryUserVar),
			varsPrefix(conf.RegistryUrlVar),
		)
		return err
	}

	_, err := fmt.Fprintf(w, PostExportOnePlainText,
		conf.OutputPath(),
		secretsPrefix(conf.KubeconfigSecret),
	)
	return err
}

func enabledOrDisabled(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}
