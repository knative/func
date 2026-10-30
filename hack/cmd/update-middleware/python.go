package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var (
	pyprojectPaths = []string{
		"templates/python/scaffolding/instanced-http/pyproject.toml",
		"templates/python/scaffolding/instanced-cloudevents/pyproject.toml",
	}

	funcPythonVersionRe = regexp.MustCompile(`"func-python==([^"]+)"`)
)

// updatePython moves func-python to the release that PyPI calls its latest.
// PyPI calls no yanked release its latest while another release exists, so
// the templates leave a release we yank, even for an older one.
func (s sources) updatePython(ctx context.Context) error {
	body, err := get(ctx, s.pypi+"/func-python/json")
	if err != nil {
		return fmt.Errorf("cannot get func-python's latest release: %w", err)
	}

	var release struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return fmt.Errorf("cannot get func-python's latest release: %w", err)
	}
	if release.Info.Version == "" {
		return errors.New("cannot get func-python's latest release: PyPI names none")
	}

	if err := updateFiles(pyprojectPaths, funcPythonVersionRe, release.Info.Version); err != nil {
		return fmt.Errorf("cannot update func-python: %w", err)
	}
	return nil
}
