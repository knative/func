//go:build exclude_graphdriver_btrfs || !cgo
// +build exclude_graphdriver_btrfs !cgo

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/openshift/source-to-image/pkg/cmd/cli"
	"k8s.io/klog/v2"

	"knative.dev/func/pkg/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/s2i"
	"knative.dev/func/pkg/scaffolding"
	"knative.dev/func/pkg/tar"
)

const middlewareFileName = "middleware-version"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs // second sigint/sigterm is treated as sigkill
		os.Exit(137)
	}()

	var cmd = unknown

	switch filepath.Base(os.Args[0]) {
	case "deploy":
		cmd = deploy
	case "scaffold":
		cmd = scaffold
	case "s2i":
		cmd = s2iCmd
	case "socat":
		cmd = socat
	case "sh":
		cmd = sh
	case "s2i-generate":
		cmd = s2iGenerate
	}

	err := cmd(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}

func unknown(_ context.Context) error {
	return fmt.Errorf("unknown command: %q", os.Args[0])
}

func socat(ctx context.Context) error {
	cmd := newSocatCmd()
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func scaffold(ctx context.Context) error {
	if len(os.Args) != 3 {
		return fmt.Errorf("expected exactly 2 positional arguments (function project path & builder)")
	}

	path := os.Args[1]
	builder := os.Args[2]

	f, err := fn.NewFunction(path)
	if err != nil {
		return fmt.Errorf("cannot load func project: %w", err)
	}

	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("cannot initialize repository: %w", err)
	}

	middlewareVersion, err := scaffolding.MiddlewareVersion(f.Root, f.Runtime, f.Invoke, embeddedRepo.FS())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: cannot get middleware version: %v\n", err)
		middlewareVersion = "<unknown>"
	}

	if err := os.WriteFile("/tekton/results/middlewareVersion", []byte(middlewareVersion), 0644); err != nil {
		return fmt.Errorf("cannot write middleware version as a result: %w", err)
	}

	if err := os.WriteFile(middlewareFileName, []byte(middlewareVersion), 0644); err != nil {
		return fmt.Errorf("cannot write middleware version as a file: %w", err)
	}

	var scaffolder fn.Scaffolder
	switch builder {
	case "pack":
		scaffolder = buildpacks.NewScaffolder(true)
	case "s2i":
		scaffolder = s2i.NewScaffolder(true)
	default:
		return fmt.Errorf("unknown builder in func-util image '%v'", builder)
	}

	return fn.New(fn.WithScaffolder(scaffolder)).Scaffold(ctx, f, "")
}

func s2iCmd(ctx context.Context) error {
	klog.InitFlags(flag.CommandLine)
	cmd := cli.CommandFor()
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func deploy(ctx context.Context) error {
	const imageDigestFileName = "image-digest"
	var err error
	var root string
	if len(os.Args) > 1 {
		root = os.Args[1]
	} else {
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine working directory: %w", err)
		}
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		return fmt.Errorf("cannot load function: %w", err)
	}

	var digestPart string
	if d, err := os.ReadFile(imageDigestFileName); err == nil {
		digestPart = "@" + string(d)
	}

	if len(os.Args) > 2 {
		f.Deploy.Image = os.Args[2] + digestPart
	}
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Image
	}
	if f.Deploy.Deployer == "" {
		f.Deploy.Deployer = knative.KnativeDeployerName
	}
	var d fn.Deployer
	switch f.Deploy.Deployer {
	case knative.KnativeDeployerName:
		d = knative.NewDeployer(
			knative.WithDeployerDecorator(deployDecorator{}),
			knative.WithDeployerVerbose(true),
		)
	case k8s.KubernetesDeployerName:
		d = k8s.NewDeployer(
			k8s.WithDeployerDecorator(deployDecorator{}),
			k8s.WithDeployerVerbose(true),
		)
	case keda.KedaDeployerName:
		d = keda.NewDeployer(
			keda.WithDeployerDecorator(deployDecorator{}),
			keda.WithDeployerVerbose(true),
		)
	default:
		return fmt.Errorf("unknown deployer: %s", f.Deploy.Deployer)
	}

	client := fn.New(fn.WithDeployer(d))
	res, err := client.Deploy(ctx, f, fn.WithDeploySkipBuildCheck(true))
	if err != nil {
		return fmt.Errorf("cannot deploy the function: %w", err)
	}

	fmt.Printf("function has been deployed\n%+v\n", res)
	return nil
}

type deployDecorator struct {
	oshDec k8s.OpenshiftMetadataDecorator
}

func (d deployDecorator) UpdateAnnotations(function fn.Function, annotations map[string]string) map[string]string {
	if k8s.IsOpenShift() {
		return d.oshDec.UpdateAnnotations(function, annotations)
	}
	return annotations
}

func (d deployDecorator) UpdateLabels(function fn.Function, labels map[string]string) map[string]string {
	if k8s.IsOpenShift() {
		return d.oshDec.UpdateLabels(function, labels)
	}
	return labels
}

func sh(ctx context.Context) error {
	if !slices.Equal(os.Args[1:], []string{"-c", "umask 0000 && exec tar -xmf -"}) {
		return fmt.Errorf("this is a fake sh (only for backward compatiblility purposes)")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot get working directory: %w", err)
	}

	unix.Umask(0)

	return tar.Extract(os.Stdin, wd)
}
