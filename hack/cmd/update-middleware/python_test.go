package main

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// updatePython moves the templates to the release that PyPI calls its
// latest, wherever it is.
func TestUpdatePython(t *testing.T) {
	tests := []struct {
		name   string
		latest string
	}{
		{name: "new major", latest: "1.0.0"},
		// PyPI's latest goes back to 0.8.0 when we yank 0.8.1.
		{name: "older after a yank", latest: "0.8.0"},
		{name: "up-to-date", latest: "0.8.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pypi := serve(t, map[string]string{
				"/pypi/func-python/json": `{"info": {"name": "func-python", "version": "` + tt.latest + `"}}`,
			})
			t.Chdir(t.TempDir())
			writeTemplates(t, pyprojectPaths, pyproject)

			assert.NilError(t, sources{pypi: pypi.URL + "/pypi"}.updatePython(t.Context()))

			assertFiles(t, pyprojectPaths, strings.Replace(pyproject, "func-python==0.8.1", "func-python=="+tt.latest, 1))
		})
	}
}

// An answer that names no release is an error, and changes no file.
func TestUpdatePythonBadAnswer(t *testing.T) {
	tests := []struct {
		name string
		body string
		err  string
	}{
		{name: "no version", body: `{"info": {}}`, err: "PyPI names none"},
		{name: "not JSON", body: `<html></html>`, err: "cannot get func-python's latest release"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pypi := serve(t, map[string]string{"/pypi/func-python/json": tt.body})
			t.Chdir(t.TempDir())
			writeTemplates(t, pyprojectPaths, pyproject)

			err := sources{pypi: pypi.URL + "/pypi"}.updatePython(t.Context())
			assert.ErrorContains(t, err, tt.err)

			assertFiles(t, pyprojectPaths, pyproject)
		})
	}
}
