package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/scaffolding"
)

const defaultPath = fn.RunDataDir + "/" + fn.BuildDir

// Scaffolder for host (OCI) builder
type Scaffolder struct {
	verbose bool
}

func NewScaffolder(verbose bool) *Scaffolder {
	return &Scaffolder{verbose: verbose}
}

// Scaffold the function so that it can be built via oci builder.
// 'path' is an optional override. Assign "" (empty string) most of the time
func (s Scaffolder) Scaffold(ctx context.Context, f fn.Function, path string) error {
	if !f.HasScaffolding() {
		if s.verbose {
			fmt.Println("Scaffolding skipped. Runtime does not support scaffolding.")
		}
		return nil
	}

	appRoot := path
	if appRoot == "" {
		appRoot = filepath.Join(f.Root, defaultPath)
	}
	if s.verbose {
		fmt.Printf("Writing host scaffolding at path '%v'\n", appRoot)
	}
	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("unable to load embedded scaffolding: %w", err)
	}
	_ = os.RemoveAll(appRoot)
	return scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS())
}
