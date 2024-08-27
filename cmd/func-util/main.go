package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/scaffolding"
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

	var cmd func(context.Context) error = unknown

	switch os.Args[0] {
	case "deploy":
		cmd = deploy
	case "scaffold":
		cmd = scaffold
	}

	err := cmd(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}

func unknown(_ context.Context) error {
	return fmt.Errorf("unknown command: " + os.Args[0])
}

func scaffold(ctx context.Context) error {

	if len(os.Args) != 2 {
		return fmt.Errorf("expected exactly one positional argument (function project path)")
	}

	path := os.Args[1]

	f, err := fn.NewFunction(path)
	if err != nil {
		return fmt.Errorf("cannot load func project: %w", err)
	}

	if f.Runtime != "go" || f.Build.Builder != "s2i" {
		// Scaffolding is for now supported/needed only for Go S2I build.
		return nil
	}

	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("cannot initialize repository: %w", err)
	}

	appRoot := filepath.Join(f.Root, ".s2i", "builds", "last")
	_ = os.RemoveAll(appRoot)

	err = scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS())
	if err != nil {
		return fmt.Errorf("cannot write the scaffolding: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(f.Root, ".s2i", "bin"), 0755); err != nil {
		return fmt.Errorf("unable to create .s2i bin dir. %w", err)
	}

	if err := os.WriteFile(filepath.Join(f.Root, ".s2i", "bin", "assemble"), []byte(s2i.GoAssembler), 0755); err != nil {
		return fmt.Errorf("unable to write go assembler. %w", err)
	}

	return nil
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
		return fmt.Errorf("cannont deploy the function: %w", err)
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
