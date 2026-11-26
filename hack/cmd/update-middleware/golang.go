package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const funcGoModule = "knative.dev/func-go"

var (
	goScaffoldingDirs = []string{
		"templates/go/scaffolding/instanced-cloudevents",
		"templates/go/scaffolding/instanced-http",
		"templates/go/scaffolding/static-cloudevents",
		"templates/go/scaffolding/static-http",
	}

	// func-go is required in a require block, or on its own require line
	// when it is a module's only requirement.
	funcGoVersionRe = regexp.MustCompile(`(?m)^\s*(?:require\s+)?knative\.dev/func-go (v\S+)`)
)

// updateGo moves func-go in the Go scaffolding modules to the release that
// the go command calls its latest. The go command calls no retracted release
// its latest while another release exists, so the templates leave a release
// we retract, even for an older one.
func updateGo(ctx context.Context) error {
	latest, err := goLatest(ctx, goScaffoldingDirs[0])
	if err != nil {
		return fmt.Errorf("cannot get func-go's latest release: %w", err)
	}

	if err := updateGoModules(ctx, goScaffoldingDirs, latest); err != nil {
		return fmt.Errorf("cannot update func-go: %w", err)
	}
	return nil
}

// updateGoModules moves func-go in the modules at dirs to version. It runs go
// get and go mod tidy in each module, so go.sum and the indirect dependencies
// follow. The modules must require the same version, which is checked before
// any module changes.
func updateGoModules(ctx context.Context, dirs []string, version string) error {
	current, err := readVersion(goMods(dirs), funcGoVersionRe)
	if err != nil {
		return err
	}
	if version == current {
		fmt.Printf("  %s is up-to-date\n", current)
		return nil
	}

	for _, dir := range dirs {
		if _, err := goCmd(ctx, dir, "get", funcGoModule+"@"+version); err != nil {
			return err
		}
		if _, err := goCmd(ctx, dir, "mod", "tidy"); err != nil {
			return err
		}
		fmt.Printf("  %s: updated %s to %s\n", dir, current, version)
	}
	return nil
}

// goMods returns the go.mod files of the modules at dirs.
func goMods(dirs []string) []string {
	paths := make([]string, len(dirs))
	for i, dir := range dirs {
		paths[i] = filepath.Join(dir, "go.mod")
	}
	return paths
}

// goLatest returns the release of func-go that the go command, run in dir,
// calls its latest.
func goLatest(ctx context.Context, dir string) (string, error) {
	out, err := goCmd(ctx, dir, "list", "-m", "-json", funcGoModule+"@latest")
	if err != nil {
		return "", err
	}

	var module struct {
		Version string
	}
	if err := json.Unmarshal(out, &module); err != nil {
		return "", err
	}
	if module.Version == "" {
		return "", errors.New("the go command names none")
	}
	return module.Version, nil
}

// goCmd runs the go command with args in dir and returns its output. Its
// error names dir and holds what the go command wrote to stderr.
func goCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: go %s: %w: %s", dir, strings.Join(args, " "), err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}
