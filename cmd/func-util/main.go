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

	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/scaffolding"
	"knative.dev/func/pkg/tar"
)

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
		return fmt.Errorf("expected exactly two positional arguments (function project path, builder)")
	}

	path := os.Args[1]
	builder := os.Args[2]

	f, err := fn.NewFunction(path)
	if err != nil {
		return fmt.Errorf("cannot load func project: %w", err)
	}

	// TODO: gauron99 - Set the builder from the passed argument if not already set in the function config.
	// This is necessary because the builder value needs to be known during scaffolding to
	// determine the correct build directory (.s2i/builds/last vs .func/builds/last), but it
	// may not be persisted to func.yaml yet. By passing it as an argument from the Tekton
	// pipeline, we ensure the correct builder is used even when the function config is incomplete.
	if f.Build.Builder == "" {
		f.Build.Builder = builder
	}

	fmt.Printf("#### scaffold: builder='%s', runtime='%s'\n", f.Build.Builder, f.Runtime)

	if f.Runtime != "go" && f.Runtime != "python" {
		// Scaffolding is for now supported/needed only for Go&Python
		return nil
	}

	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("cannot initialize repository: %w", err)
	}

	appRoot := filepath.Join(f.Root, ".s2i", "builds", "last")
	if f.Build.Builder != "s2i" {
		// TODO: gauron99 - change this completely
		appRoot = filepath.Join(f.Root, ".func", "builds", "last")
	}
	fmt.Printf("appRoot is '%s'\n", appRoot)
	_ = os.RemoveAll(appRoot)

	// build step now includes scaffolding for go-pack
	err = scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS())
	if err != nil {
		return fmt.Errorf("cannot write the scaffolding: %w", err)
	}

	if f.Build.Builder != "s2i" {
		return nil
	}

	// add s2i specific changes

	if err := os.MkdirAll(filepath.Join(f.Root, ".s2i", "bin"), 0755); err != nil {
		return fmt.Errorf("unable to create .s2i bin dir. %w", err)
	}

	var asm string
	switch f.Runtime {
	case "go":
		asm = s2i.GoAssembler
	case "python":
		asm = s2i.PythonAssembler
	default:
		panic("unreachable")
	}

	if err := os.WriteFile(filepath.Join(f.Root, ".s2i", "bin", "assemble"), []byte(asm), 0755); err != nil {
		return fmt.Errorf("unable to write go assembler. %w", err)
	}
	return nil
}

func s2iCmd(ctx context.Context) error {
	klog.InitFlags(flag.CommandLine)
	cmd := cli.CommandFor()
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func deploy(ctx context.Context) error {
	var err error
	deployer := knative.NewDeployer(
		knative.WithDeployerVerbose(true),
		knative.WithDeployerDecorator(deployDecorator{}))

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
	if len(os.Args) > 2 {
		f.Deploy.Image = os.Args[2]
	}
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Image
	}

	res, err := deployer.Deploy(ctx, f)
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
