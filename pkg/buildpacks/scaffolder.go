package buildpacks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/scaffolding"
)

const defaultPath = fn.RunDataDir + "/" + fn.BuildDir

// Scaffolder for buildpacks builder
type Scaffolder struct {
	verbose bool
}

func NewScaffolder(verbose bool) *Scaffolder {
	return &Scaffolder{verbose: verbose}
}

// Scaffold writes scaffolding for the function's runtime to the target path
// using embedded templates. Pass "" for path to use the default (.func/build/).
// Runtime-specific processing is applied after scaffolding is written.
func (s Scaffolder) Scaffold(ctx context.Context, f fn.Function, path string) error {
	switch f.Runtime {
	case "go", "python":
	default:
		if s.verbose {
			fmt.Println("Scaffolding skipped. Currently available for runtimes go, python")
		}
		return nil
	}

	appRoot := path
	if appRoot == "" {
		appRoot = filepath.Join(f.Root, defaultPath)
	}
	if s.verbose {
		fmt.Printf("Writing %s buildpacks scaffolding at path '%v'\n", f.Runtime, appRoot)
	}

	embeddedRepo, err := fn.NewRepository("", "")
	if err != nil {
		return fmt.Errorf("unable to load embedded scaffolding: %w", err)
	}
	if err = os.RemoveAll(appRoot); err != nil {
		return fmt.Errorf("cannot clean scaffolding directory: %w", err)
	}
	if err = scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS()); err != nil {
		return err
	}

	if f.Runtime != "python" {
		return nil
	}
	// Python specific: patch pyproject.toml for Poetry and write Procfile.
	//
	// Add procfile for python+pack scaffolding. This will be used for buildpacks
	// build/run phases to tell pack what to run. Note that this is not for
	// detection as pre-buildpack runs only at build phase.
	// Variable BP_PROCFILE_DEFAULT_PROCESS (see builder.go) is used for detection
	// for local deployment.
	if err = patchPyprojectForPack(filepath.Join(appRoot, "pyproject.toml")); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(appRoot, "Procfile"),
		[]byte(PythonScaffoldingProcfile()),
		os.FileMode(0644),
	)
}

// patchPyprojectForPack applies pack-specific modifications to the template
// pyproject.toml:
//   - Replaces {root:uri}/f with ./f — Poetry doesn't understand hatchling's
//     {root:uri} context variable. The inline buildpack creates a symlink
//     f -> fn/ so ./f resolves correctly.
//   - Appends [tool.poetry.dependencies] — Poetry needs this section for
//     dependency solving
func patchPyprojectForPack(pyprojectPath string) error {
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return fmt.Errorf("cannot read pyproject.toml for patching: %w", err)
	}
	content := strings.Replace(string(data), "{root:uri}/f", "./f", 1)
	content += "[tool.poetry.dependencies]\npython = \">=3.9,<4.0\"\nfunction = { path = \"f\", develop = true }\n"
	if err = os.WriteFile(pyprojectPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write patched pyproject.toml: %w", err)
	}
	return nil
}

// PythonScaffoldingProcfile returns the Procfile content that tells the
// buildpack how to start the service.
func PythonScaffoldingProcfile() string {
	return "web: python -m service.main\n"
}

// pythonScaffoldScript returns a bash script for use as an inline buildpack.
// The script rearranges user code into fn/ and moves pre-written scaffolding
// from .func/build/ (populated by Scaffold) to the workspace root.
func pythonScaffoldScript() string {
	return `#!/bin/bash
set -eo pipefail

# Move user code into fn/ subdirectory, preserving infrastructure entries
shopt -s dotglob
mkdir -p fn
for item in *; do
  case "$item" in
    fn|.func|.git|.gitignore|func.yaml) continue ;;
  esac
  mv "$item" fn/
done
shopt -u dotglob

# Move scaffolding from .func/build/ to root
mv .func/build/* .
rm -rf .func

# Create symlink so ./f in pyproject.toml resolves to fn/
# -n: treat existing symlink as file, not follow it to directory
ln -snf fn f
`
}
