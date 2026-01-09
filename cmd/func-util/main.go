//go:build exclude_graphdriver_btrfs || !cgo
// +build exclude_graphdriver_btrfs !cgo

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/openshift/source-to-image/pkg/cmd/cli"
	"k8s.io/klog/v2"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/s2i"
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

const middlewareFileName = "middleware-version"

func scaffold(ctx context.Context) error {
	logger := log.New(os.Stderr, "scaffold:", log.LstdFlags)
	logger.Printf("args: %#v", os.Args)

	if len(os.Args) != 2 {
		return fmt.Errorf("expected exactly one positional argument (function project path)")
	}

	path := os.Args[1]

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
		return fmt.Errorf("cannot get middleware version: %w", err)
	}

	logger.Println("middleware:", middlewareVersion)

	if err := os.WriteFile("middleware-version", []byte(middlewareVersion), 0644); err != nil {
		return fmt.Errorf("cannot write middleware version as a result: %w", err)
	}

	if f.Runtime != "go" && f.Runtime != "python" {
		// Scaffolding is for now supported/needed only for Go/Python.
		return nil
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

const imageDigestFileName = "image-digest"

func deploy(ctx context.Context) error {
	logger := log.New(os.Stderr, "deploy:", log.LstdFlags)
	logger.Printf("args: %#v", os.Args)

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

	var digestPart string
	d, err := os.ReadFile(imageDigestFileName)
	if err == nil {
		digestPart = "@" + string(d)
	}

	if len(os.Args) > 2 {
		f.Deploy.Image = os.Args[2] + digestPart
	}
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Image
	}

	logger.Printf("fn: %#v", f)

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
