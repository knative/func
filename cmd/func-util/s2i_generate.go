//go:build exclude_graphdriver_btrfs || !cgo
// +build exclude_graphdriver_btrfs !cgo

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/scm/git"
	"github.com/spf13/cobra"

	fn "knative.dev/func/pkg/functions"
)

func s2iGenerate(ctx context.Context) error {
	cmd := newS2IGenerateCmd()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		return fmt.Errorf("cannot s2i generate: %w", err)
	}
	return nil
}

type genConfig struct {
	target         string
	pathContext    string
	builderImage   string
	registry       string
	imageScriptUrl string
	logLevel       string
	envVars        []string
}

func newS2IGenerateCmd() *cobra.Command {
	var config genConfig

	genCmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			config.envVars = args
			return runS2IGenerate(cmd.Context(), config)
		},
	}
	genCmd.Flags().StringVar(&config.target, "target", "/gen-source", "")
	genCmd.Flags().StringVar(&config.pathContext, "path-context", ".", "")
	genCmd.Flags().StringVar(&config.builderImage, "builder-image", "", "")
	genCmd.Flags().StringVar(&config.registry, "registry", "", "")
	genCmd.Flags().StringVar(&config.imageScriptUrl, "image-script-url", "image:///usr/libexec/s2i", "")
	genCmd.Flags().StringVar(&config.logLevel, "log-level", "0", "")

	return genCmd
}

func runS2IGenerate(ctx context.Context, c genConfig) error {

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot get working directory: %w", err)
	}

	funcRoot := filepath.Join(wd, c.pathContext)

	// replace registry in func.yaml
	f, err := fn.NewFunction(funcRoot)
	if err != nil {
		return fmt.Errorf("cannot load function: %w", err)
	}
	f.Registry = c.registry
	err = f.Write()
	if err != nil {
		return fmt.Errorf("cannot write function: %w", err)
	}

	// append node_modules into .s2iignore
	s2iIgnorePath := filepath.Join(funcRoot, ".s2iignore")
	if fi, _ := os.Stat(s2iIgnorePath); fi != nil {
		var file *os.File

		file, err = os.OpenFile(s2iIgnorePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("cannot open s2i ignore file for append: %w", err)
		}
		defer func(file *os.File) {
			_ = file.Close()

		}(file)

		_, err = file.Write([]byte("\nnode_modules"))
		if err != nil {
			return fmt.Errorf("cannot append node_modules directory to s2i ignore file: %w", err)
		}
	}

	// prepare envvars
	var envs = make([]api.EnvironmentSpec, 0, len(c.envVars))
	for _, e := range c.envVars {
		var es api.EnvironmentSpec
		part := strings.SplitN(e, "=", 2)
		switch len(part) {
		case 1:
			es.Name = part[0]
		case 2:
			es.Name = part[0]
			es.Value = part[1]
		default:
			continue
		}
		if es.Name != "" {
			envs = append(envs, es)
		}
	}

	s2iConfig := api.Config{
		Source: &git.URL{
			URL:  url.URL{Path: funcRoot},
			Type: git.URLTypeLocal,
		},
		BuilderImage:    c.builderImage,
		ImageScriptsURL: c.imageScriptUrl,
		KeepSymlinks:    true,
		Environment:     envs,
		AsDockerfile:    filepath.Join(c.target, "Dockerfile.gen"),
	}

	builder, _, err := strategies.Strategy(nil, &s2iConfig, build.Overrides{})
	if err != nil {
		return fmt.Errorf("cannot create builder: %w", err)
	}

	_, err = builder.Build(&s2iConfig)
	if err != nil {
		return fmt.Errorf("cannot build: %w", err)
	}

	return nil
}
