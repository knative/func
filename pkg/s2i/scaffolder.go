package s2i

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/scaffolding"
)

const defaultPath = fn.RunDataDir + "/" + fn.BuildDir

// Scaffolder for S2I builder
type Scaffolder struct {
	verbose bool
}

func NewScaffolder(verbose bool) *Scaffolder {
	return &Scaffolder{verbose: verbose}
}

// Scaffold the function so that it can be built via s2i builder.
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
		fmt.Printf("Writing s2i scaffolding at path '%v'\n", appRoot)
	}
	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("unable to load embedded scaffolding: %w", err)
	}
	_ = os.RemoveAll(appRoot)
	if err := scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS()); err != nil {
		return err
	}

	// Write out an S2I assembler script if the runtime needs to override the
	// one provided in the S2I image.
	assemble, err := assembler(f)
	if err != nil {
		return err
	}
	if assemble != "" {
		binDir := filepath.Join(appRoot, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return fmt.Errorf("unable to create scaffolding bin dir. %w", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "assemble"), []byte(assemble), 0700); err != nil {
			return fmt.Errorf("unable to write assembler script. %w", err)
		}
	}
	return nil
}
