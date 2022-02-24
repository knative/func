package cmd

import (
	"fmt"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"

	"github.com/spf13/cobra"
)

// NewExportCmd export a func.yaml to CRD-like resource(s).
func NewExportCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Function File to other formats",
		Long: `
NAME
	{{.Name}} export - Export a Function configuration file.

SYNOPSIS
	{{.Name}} export [-t|--type] [-f|--file]  [-v|--verbose] 

DESCRIPTION
	Export a Function configuration file (func.yaml) into a different format

	  $ {{.Name}} export -t crd -f my-resource.yaml

	`,
		SuggestFor: []string{"exprot", "exports", "expor"},
		PreRunE:    bindEnv("type", "path", "file"),
	}

	// Flags

	cmd.Flags().StringP("type", "t", "crd", "Format for the exported func.yaml file. Default: crd")
	cmd.Flags().StringP("file", "", cwd(), "Path to a file to export the func.yaml file")
	setPathFlag(cmd)
	// Help Action
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		runCreateHelp(cmd, args, newClient)
	})

	// Run Action
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExport(cmd, args, newClient)
	}

	return cmd
}

// Run Export
func runExport(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Load function using build config

	config := newExportConfig()

	function, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function. Please create one at this path before exporting", config.Path)
	}

	funcCRD := fn.NewFunctionCRD()
	funcCRD.Root = function.Root // I am doing this so I can write the func
	funcCRD.ObjectMeta.Name = function.Name
	funcCRD.ObjectMeta.Annotations["version"] = function.Version
	funcCRD.ObjectMeta.Annotations["runtime"] = function.Runtime
	funcCRD.ObjectMeta.Annotations["invocation"] = function.Invocation.Format
	funcCRD.ObjectMeta.Namespace = function.Namespace
	funcCRD.Spec.HealthEndpoints = function.HealthEndpoints
	funcCRD.Spec.Image = function.Image
	funcCRD.Write()
	// Confirm
	fmt.Fprintf(cmd.OutOrStderr(), "Exporting %v Function in %v\n", funcCRD.ObjectMeta.Name, config.File)

	return nil
}

type exportConfig struct {
	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Type format used to export the func.yaml file
	Type string

	// File used to write the exported version of the func.yaml file
	File string
}

func newExportConfig() exportConfig {
	return exportConfig{
		File: viper.GetString("file"),
		Type: viper.GetString("type"),
		Path: viper.GetString("path"),
	}
}

// Run Help
func runExportHelp(cmd *cobra.Command, args []string, newClient ClientFactory) {
	// Error-tolerant implementation:
	// Help can not fail when creating the client config (such as on invalid
	// flag values) because help text is needed in that situation.   Therefore
	// this implementation must be resilient to cfg zero value.
	failSoft := func(err error) {
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "error: help text may be partial: %v", err)
		}
	}

	tpl := createHelpTemplate(cmd)

	cfg, err := newCreateConfig(args, newClient)
	failSoft(err)

	client := newClient(createConfigToClientOptions(cfg))

	options, err := runtimeTemplateOptions(client) // human-friendly
	failSoft(err)

	var data = struct {
		Options string
		Name    string
	}{
		Options: options,
		Name:    cmd.Root().Name(),
	}
	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}
