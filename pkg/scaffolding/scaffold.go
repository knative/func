package scaffolding

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
	"knative.dev/func/pkg/filesystem"
)

// Write scaffolding to a given path
//
// Scaffolding is a language-level operation which first detects the method
// signature used by the function's source code and then writes the
// appropriate scaffolding.
//
// NOTE: Scaffolding is not per-template, because a template is merely an
// example starting point for a Function implementation and should have no
// bearing on the shape that function can eventually take.  The language,
// and optionally invocation hint (For cloudevents) are used for this.  For
// example, there can be multiple templates which exemplify a given method
// signature, and the implementation can be switched at any time by the author.
// Language, by contrast, is fixed at time of initialization.
//
//	out:     the path to output scaffolding
//	src:      the path to the source code to scaffold
//	runtime: the expected runtime of the target source code "go", "node" etc.
//	invoke:  the optional invocation hint (default "http")
//	fs:      filesystem which contains scaffolding at '[runtime]/scaffolding'
//	         (exclusive with 'repo')
func Write(out, src, runtime, invoke string, fs filesystem.Filesystem) (err error) {
	// detect the signature of the source code in the given location, presuming
	// a runtime and invocation hint (default "http")
	s, err := detectSignature(src, runtime, invoke)
	if err != nil {
		return err
	}

	// Filesystem Required
	// This is a defensive check used to allow for simple tests which can
	// omit providing a fileystem by just expecting this error.
	if fs == nil {
		return ErrFilesystemRequired
	}

	// Path in the filesystem at which scaffolding is expected to exist
	d := fmt.Sprintf("%v/scaffolding/%v", runtime, s.String()) // fs uses / on all OSs
	if _, err := fs.Stat(d); err != nil {
		return ErrScaffoldingNotFound
	}

	// Copy from d -> out from the filesystem
	if err := filesystem.CopyFromFS(d, out, fs); err != nil {
		return ScaffoldingError{"filesystem copy failed", err}
	}

	// Slightly prouder moment
	if runtime == "go" {
		if err := patchScaffolding(src, out); err != nil {
			return fmt.Errorf("failed to patch scaffolding:%w", err)
		}
	}

	// Copy the certs from the filesystem to the build directory
	if _, err := fs.Stat("certs"); err != nil {
		return ScaffoldingError{"certs directory not found in filesystem", err}
	}
	if err := filesystem.CopyFromFS("certs", out, fs); err != nil {
		return ScaffoldingError{"certs copy failed", err}
	}

	// Replace the 'f' link of the scaffolding (which is now incorrect) to
	// link to the function's root.
	rel, err := filepath.Rel(out, src)
	if err != nil {
		return ScaffoldingError{"error determining relative path to function source", err}
	}
	link := filepath.Join(out, "f")
	_ = os.Remove(link)
	if err = os.Symlink(rel, link); err != nil {
		return fmt.Errorf("error linking scaffolding to source %w", err)
	}
	return
}

// detectSignature returns the Signature of the source code at the given
// location assuming a provided runtime and invocation hint.
func detectSignature(src, runtime, invoke string) (s Signature, err error) {
	d, err := newDetector(runtime)
	if err != nil {
		return UnknownSignature, err
	}
	static, instanced, err := d.Detect(src)
	if err != nil {
		return
	}
	// Function must implement either a static handler or the instanced handler
	// but not both.
	if static && instanced {
		return s, fmt.Errorf("function may not implement both the static and instanced method signatures simultaneously")
	} else if !static && !instanced {
		return s, fmt.Errorf("function does not implement any known method signatures or does not compile")
	} else {
		return toSignature(instanced, invoke), nil
	}
}

// patch scaffolding main.go, go.mod and go.sum based on the users function
func patchScaffolding(src, out string) error {
	goModPath := filepath.Join(src, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("cannot read function go.mod: %w", err)
	}
	goMod, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return fmt.Errorf("cannot parse function go.mod at: %w", err)
	}
	if goMod.Module == nil {
		return fmt.Errorf("cannot parse function go.mod: module not found")
	}

	moduleName := goMod.Module.Mod.Path

	if err := patchGoMod(out, moduleName, goMod.Go, goMod.Require); err != nil {
		return err
	}
	if err := mergeGoSum(src, out); err != nil {
		return err
	}
	return patchMain(out, moduleName)
}

func patchGoMod(dir, moduleName string, goDirective *modfile.Go, funcRequires []*modfile.Require) error {
	path := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read scaffolding go.mod: %w", err)
	}
	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return fmt.Errorf("cannot parse scaffolding go.mod: %w", err)
	}

	if goDirective != nil && goDirective.Version != "" {
		if err := f.AddGoStmt(goDirective.Version); err != nil {
			return fmt.Errorf("cannot set go version: %w", err)
		}
	}
	if err := f.DropReplace("function", ""); err != nil {
		return fmt.Errorf("cannot drop replace: %w", err)
	}
	if err := f.AddReplace(moduleName, "", "./f", ""); err != nil {
		return fmt.Errorf("cannot add replace: %w", err)
	}
	if err := f.DropRequire("function"); err != nil {
		return fmt.Errorf("cannot drop require: %w", err)
	}
	if err := f.AddRequire(moduleName, "v0.0.0-00010101000000-000000000000"); err != nil {
		return fmt.Errorf("cannot add require: %w", err)
	}

	// Merge function's dependencies that aren't already in the scaffolding
	// go.mod. This ensures transitive requires (e.g. private dependencies)
	// are present without downgrading versions the scaffolding already has.
	existing := make(map[string]bool, len(f.Require))
	for _, req := range f.Require {
		existing[req.Mod.Path] = true
	}
	for _, req := range funcRequires {
		if existing[req.Mod.Path] {
			continue
		}
		if err := f.AddRequire(req.Mod.Path, req.Mod.Version); err != nil {
			return fmt.Errorf("cannot add require %s: %w", req.Mod.Path, err)
		}
	}

	formatted, err := f.Format()
	if err != nil {
		return fmt.Errorf("cannot format scaffolding go.mod: %w", err)
	}
	return os.WriteFile(path, formatted, 0644)
}

// mergeGoSum appends the function's go.sum entries into the scaffolding's
// go.sum so that all transitive dependency checksums are available at build
// time. Duplicate entries are harmless; Go tooling ignores them.
func mergeGoSum(src, out string) error {
	funcGoSum := filepath.Join(src, "go.sum")
	data, err := os.ReadFile(funcGoSum)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cannot read function go.sum: %w", err)
	}

	scaffoldGoSum := filepath.Join(out, "go.sum")
	f, err := os.OpenFile(scaffoldGoSum, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot open scaffolding go.sum: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("cannot append to scaffolding go.sum: %w", err)
	}
	return nil
}

func patchMain(out, moduleName string) error {
	targetMainPath := filepath.Join(out, "main.go")
	targetMainData, err := os.ReadFile(targetMainPath)
	if err != nil {
		return fmt.Errorf("cannot read scaffolding main: %w", err)
	}
	targetMainData = bytes.ReplaceAll(targetMainData, []byte("function"), []byte(moduleName))
	return os.WriteFile(targetMainPath, targetMainData, 0644)
}
