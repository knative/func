package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

const goMod = `module s

replace function => ./f

go 1.25.0

require (
	function v0.0.0-00010101000000-000000000000
	knative.dev/func-go v0.23.0
)
`

func TestFuncGoVersionRe(t *testing.T) {
	for _, mod := range []string{goMod, "module s\n\ngo 1.25.0\n\nrequire knative.dev/func-go v0.23.0\n"} {
		m := funcGoVersionRe.FindStringSubmatch(mod)
		assert.Assert(t, m != nil, mod)
		assert.Equal(t, m[1], "v0.23.0")
	}
}

// An up-to-date module must not run the go command, so this needs no network.
func TestUpdateGoModulesUpToDate(t *testing.T) {
	dirs := writeGoMods(t, goMod, goMod)

	assert.NilError(t, updateGoModules(t.Context(), dirs, "v0.23.0"))

	for _, path := range goMods(dirs) {
		got, err := os.ReadFile(path)
		assert.NilError(t, err)
		assert.Equal(t, string(got), goMod, path)
	}
}

// When one module cannot be updated, the go command runs in none, so this
// needs no network either.
func TestUpdateGoModulesChangesNothingOnError(t *testing.T) {
	tests := []struct {
		name   string
		goMods []string
		err    string
	}{
		{
			name:   "no match",
			goMods: []string{goMod, "module s\n\ngo 1.25.0\n"},
			err:    "no match",
		},
		{
			name:   "modules disagree",
			goMods: []string{goMod, strings.Replace(goMod, "v0.23.0", "v0.22.0", 1)},
			err:    "has v0.23.0, but",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := writeGoMods(t, tt.goMods...)

			err := updateGoModules(t.Context(), dirs, "v0.24.0")
			assert.ErrorContains(t, err, tt.err)

			for i, path := range goMods(dirs) {
				got, err := os.ReadFile(path)
				assert.NilError(t, err)
				assert.Equal(t, string(got), tt.goMods[i], path)
			}
		})
	}
}

// writeGoMods writes each of content as the go.mod of its own module and
// returns the modules' dirs.
func writeGoMods(t *testing.T, content ...string) []string {
	t.Helper()

	var dirs []string
	for _, c := range content {
		dir := t.TempDir()
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(c), 0644))
		dirs = append(dirs, dir)
	}
	return dirs
}

// updateGo moves every module to the release that the go command calls
// func-go's latest: a new major, or an older release when the newer ones are
// retracted. go.sum follows, and go mod tidy drops the old version from it.
func TestUpdateGo(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		retract  string
		want     string
	}{
		{
			name:     "new major",
			versions: []string{"0.22.0", "0.23.0", "0.24.0", "1.0.0", "1.1.0-rc.1"},
			want:     "v1.0.0",
		},
		{
			// v0.23.1 retracts the templates' v0.23.0, and itself.
			name:     "older after a retract",
			versions: []string{"0.22.0", "0.23.0", "0.23.1"},
			retract:  "retract [v0.23.0, v0.23.1]",
			want:     "v0.22.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goProxy(t, tt.retract, tt.versions...)
			t.Chdir(t.TempDir())
			writeGoModules(t, goScaffoldingDirs)

			assert.NilError(t, updateGo(t.Context()))

			for _, dir := range goScaffoldingDirs {
				mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
				assert.NilError(t, err)
				assert.Assert(t, strings.Contains(string(mod), "knative.dev/func-go "+tt.want+"\n"), "%s:\n%s", dir, mod)

				sum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
				assert.NilError(t, err)
				assert.Assert(t, strings.Contains(string(sum), "knative.dev/func-go "+tt.want+" h1:"), "%s:\n%s", dir, sum)
				assert.Assert(t, !strings.Contains(string(sum), "v0.23.0"), "%s:\n%s", dir, sum)
			}
		})
	}
}

// A failed go command fails the update, and its error names the module and
// holds what go said.
func TestUpdateGoFailsOnGoError(t *testing.T) {
	goProxy(t, "", "0.23.0", "0.24.0")
	t.Chdir(t.TempDir())
	writeGoModules(t, goScaffoldingDirs)
	// No module provides this package, so go mod tidy fails in the last
	// module.
	last := goScaffoldingDirs[len(goScaffoldingDirs)-1]
	assert.NilError(t, os.WriteFile(filepath.Join(last, "missing.go"), []byte("package main\n\nimport _ \"example.com/missing\"\n"), 0644))

	err := updateGo(t.Context())
	assert.ErrorContains(t, err, last+": go mod tidy")
	assert.ErrorContains(t, err, "example.com/missing")
}

// goProxy serves a Go module proxy that holds a fake func-go at each of
// versions, and points the go command at it, with a module cache of its own.
// Like the real func-go, the fake has no package at its root, only in http/.
// The go.mod of every version holds retract, if it is not "".
func goProxy(t *testing.T, retract string, versions ...string) {
	t.Helper()

	root := t.TempDir()
	dir := filepath.Join(root, funcGoModule, "@v")
	assert.NilError(t, os.MkdirAll(dir, 0755))

	mod := "module knative.dev/func-go\n\ngo 1.22\n"
	if retract != "" {
		mod += "\n" + retract + "\n"
	}
	var list strings.Builder
	for _, v := range versions {
		v = "v" + v
		fmt.Fprintln(&list, v)

		info := fmt.Sprintf(`{"Version": %q, "Time": "2026-01-01T00:00:00Z"}`, v)
		assert.NilError(t, os.WriteFile(filepath.Join(dir, v+".info"), []byte(info), 0644))
		assert.NilError(t, os.WriteFile(filepath.Join(dir, v+".mod"), []byte(mod), 0644))

		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for name, content := range map[string]string{"go.mod": mod, "http/http.go": "package http\n"} {
			f, err := z.Create(funcGoModule + "@" + v + "/" + name)
			assert.NilError(t, err)
			_, err = f.Write([]byte(content))
			assert.NilError(t, err)
		}
		assert.NilError(t, z.Close())
		assert.NilError(t, os.WriteFile(filepath.Join(dir, v+".zip"), buf.Bytes(), 0644))
	}
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "list"), []byte(list.String()), 0644))

	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(srv.Close)

	t.Setenv("GOPROXY", srv.URL)
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GONOSUMDB", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	// The go command makes the module cache read-only, which t.TempDir
	// cannot remove.
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOMODCACHE", t.TempDir())
}

// writeGoModules writes a module at each of dirs, under the current dir,
// that requires func-go v0.23.0 and imports its http package, and tidies it.
func writeGoModules(t *testing.T, dirs []string) {
	t.Helper()

	for _, dir := range dirs {
		assert.NilError(t, os.MkdirAll(dir, 0755))
		mod := "module example.com/f\n\ngo 1.22\n\nrequire knative.dev/func-go v0.23.0\n"
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644))
		main := "package main\n\nimport _ \"knative.dev/func-go/http\"\n\nfunc main() {}\n"
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644))
		_, err := goCmd(t.Context(), dir, "mod", "tidy")
		assert.NilError(t, err)
	}
}

func TestGoTemplatesMatch(t *testing.T) {
	t.Chdir("../../..")

	checkTemplates(t, goMods(goScaffoldingDirs), funcGoVersionRe)
}
