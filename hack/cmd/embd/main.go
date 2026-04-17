// Package main implements the embd/unembd tool for reversibly mangling
// a template directory tree to satisfy go:embed constraints.
//
// The go:embed directive has two relevant limitations:
//  1. It silently skips directories containing a go.mod file.
//  2. It silently drops symlinks.
//
// To work around these, the templates/ tree stores:
//   - go.mod / go.sum renamed to go.mod.embd / go.sum.embd
//   - symlinks replaced by <name>.symlink plain-text files containing the target
//   - Go source files prefixed with "//go:build embd" so the Go toolchain
//     does not compile them when building the main module
//
// This tool can temporarily undo ("unembd") that mangling so that normal
// Go tooling (go mod tidy, go get, go test …) can operate on the directory,
// and then redo ("embd") it to restore the go:embed-friendly form.
//
// USAGE:
//
//	go run ./hack/cmd/embd embd   <dir>   # mangle dir tree for go:embed
//	go run ./hack/cmd/embd unembd <dir>   # restore dir tree for Go tooling
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"knative.dev/func/pkg/filesystem"
)

func main() {
	if len(os.Args) != 3 {
		usage()
		os.Exit(1)
	}
	cmd, dir := os.Args[1], os.Args[2]
	var err error
	switch cmd {
	case "embd":
		err = embd(dir)
	case "unembd":
		err = unembd(dir)
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "embd: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: embd embd|unembd <dir>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  embd   <dir>  mangle dir tree for go:embed")
	fmt.Fprintln(os.Stderr, "  unembd <dir>  restore dir tree for Go tooling")
}

// embd walks dir and applies the mangling:
//   - go.mod → go.mod.embd
//   - go.sum → go.sum.embd
//   - symlink foo → foo.symlink plain-text file with target
//   - *.go files → add "//go:build embd" constraint
func embd(dir string) error {
	// We need two passes: first handle non-.go files (renames + symlinks),
	// then .go files. Doing it in one walk risks double-processing.
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()

		// Handle symlinks first (before they are resolved by WalkDir).
		if d.Type()&fs.ModeSymlink != 0 {
			return embdSymlink(path)
		}

		switch name {
		case "go.mod", "go.sum":
			return os.Rename(path, path+filesystem.EmbdSuffix)
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(name, ".go") {
			return addBuildTag(path)
		}
		return nil
	})
}

// embdSymlink converts a real symlink at path into a .symlink marker file.
func embdSymlink(path string) error {
	target, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("readlink %s: %w", path, err)
	}
	markerPath := path + filesystem.SymlinkSuffix
	// Write target + trailing newline (satisfies EOF-newline linting).
	if err := os.WriteFile(markerPath, []byte(target+"\n"), 0644); err != nil {
		return fmt.Errorf("write %s: %w", markerPath, err)
	}
	return os.Remove(path)
}

// addBuildTag reads path, adds the embd build constraint, and writes back.
func addBuildTag(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tagged := filesystem.AddEmbdBuildTag(src)
	if len(tagged) == len(src) {
		// Unchanged — AddEmbdBuildTag determined tag already present.
		return nil
	}
	return os.WriteFile(path, tagged, 0644)
}

// unembd walks dir and reverses the mangling:
//   - go.mod.embd → go.mod
//   - go.sum.embd → go.sum
//   - foo.symlink plain-text → real symlink foo
//   - *.go files → strip "//go:build embd" constraint
func unembd(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip actual symlinks (should not exist after embd, but be defensive).
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		name := d.Name()

		switch {
		case strings.HasSuffix(name, filesystem.EmbdSuffix):
			bare := strings.TrimSuffix(path, filesystem.EmbdSuffix)
			return os.Rename(path, bare)
		case strings.HasSuffix(name, filesystem.SymlinkSuffix):
			return unembdSymlink(path)
		case strings.HasSuffix(name, ".go"):
			return stripBuildTag(path)
		}
		return nil
	})
}

// unembdSymlink reads the .symlink marker at path and recreates the real symlink.
func unembdSymlink(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	target := strings.TrimRight(string(raw), "\n\r")
	linkPath := strings.TrimSuffix(path, filesystem.SymlinkSuffix)
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("symlink %s → %s: %w", linkPath, target, err)
	}
	return os.Remove(path)
}

// stripBuildTag reads path, strips the embd build constraint, and writes back.
func stripBuildTag(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	stripped := filesystem.StripEmbdBuildTag(src)
	if len(stripped) == len(src) {
		return nil
	}
	// Preserve original file permissions.
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, stripped, fi.Mode())
}
