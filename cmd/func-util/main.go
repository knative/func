package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
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
