package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// run tries every middleware, even after one fails, and names the ones that
// failed.
func TestRun(t *testing.T) {
	var tried []string
	update := func(runtime string, err error) func(context.Context) error {
		return func(context.Context) error {
			tried = append(tried, runtime)
			return err
		}
	}

	err := run(t.Context(), []middleware{
		{"a", "A", update("a", errors.New("source is down"))},
		{"b", "B", update("b", nil)},
		{"c", "C", update("c", errors.New("no match"))},
	})
	assert.Error(t, err, "failed middleware updates: a, c")
	assert.DeepEqual(t, tried, []string{"a", "b", "c"})

	assert.NilError(t, run(t.Context(), []middleware{{"b", "B", update("b", nil)}}))
}

const quarkusPom = `<project>
  <properties>
    <quarkus.platform.version>3.39.5</quarkus.platform.version>
  </properties>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <version>${quarkus.platform.version}</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>
`

const springBootPom = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<project>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.5.12</version>
    <relativePath/>
  </parent>
  <groupId>com.example.events</groupId>
  <artifactId>function</artifactId>
  <version>0.0.1-SNAPSHOT</version>
  <properties>
    <spring-cloud.version>2025.0.1</spring-cloud.version>
  </properties>
</project>
`

const pyproject = `[project]
name = "function"
dependencies = [
    "func-python==0.8.1",
    "httpx",
]
`

func TestUpdateFiles(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		re       *regexp.Regexp
		version  string
		from, to string // the line holding the version, before and after
	}{
		{
			name:    "func-python",
			content: pyproject,
			re:      funcPythonVersionRe,
			version: "1.0.0",
			from:    `"func-python==0.8.1"`,
			to:      `"func-python==1.0.0"`,
		},
		{
			name:    "quarkus",
			content: quarkusPom,
			re:      quarkusVersionRe,
			version: "3.40.1.1",
			from:    "<quarkus.platform.version>3.39.5</quarkus.platform.version>",
			to:      "<quarkus.platform.version>3.40.1.1</quarkus.platform.version>",
		},
		{
			name:    "spring boot parent",
			content: springBootPom,
			re:      springBootVersionRe,
			version: "3.5.16",
			from:    "<version>3.5.12</version>",
			to:      "<version>3.5.16</version>",
		},
		{
			name:    "spring cloud",
			content: springBootPom,
			re:      springCloudVersionRe,
			version: "2025.0.3",
			from:    "<spring-cloud.version>2025.0.1</spring-cloud.version>",
			to:      "<spring-cloud.version>2025.0.3</spring-cloud.version>",
		},
		{
			name:    "up-to-date",
			content: quarkusPom,
			re:      quarkusVersionRe,
			version: "3.39.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := writeFiles(t, tt.content, tt.content)

			assert.NilError(t, updateFiles(paths, tt.re, tt.version))

			want := strings.Replace(tt.content, tt.from, tt.to, 1)
			for _, path := range paths {
				got, err := os.ReadFile(path)
				assert.NilError(t, err)
				assert.Equal(t, string(got), want, path)
			}
		})
	}
}

// When one file cannot be updated, no file is written, so the templates
// never end up on two versions.
func TestUpdateFilesWritesNothingOnError(t *testing.T) {
	tests := []struct {
		name    string
		content []string
		err     string
	}{
		{
			name:    "no match",
			content: []string{quarkusPom, pyproject},
			err:     "no match",
		},
		{
			name:    "two matches",
			content: []string{quarkusPom, strings.Replace(quarkusPom, "</properties>", "  <quarkus.platform.version>3.39.5</quarkus.platform.version>\n  </properties>", 1)},
			err:     "2 matches",
		},
		{
			name:    "files disagree",
			content: []string{quarkusPom, strings.Replace(quarkusPom, "3.39.5", "3.40.1.1", 1)},
			err:     "has 3.39.5, but",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := writeFiles(t, tt.content...)

			err := updateFiles(paths, quarkusVersionRe, "3.40.1.2")
			assert.ErrorContains(t, err, tt.err)

			for i, path := range paths {
				got, err := os.ReadFile(path)
				assert.NilError(t, err)
				assert.Equal(t, string(got), tt.content[i], path)
			}
		})
	}
}

// writeFiles writes each of content to its own file and returns their paths.
func writeFiles(t *testing.T, content ...string) []string {
	t.Helper()

	dir := t.TempDir()
	var paths []string
	for i, c := range content {
		path := filepath.Join(dir, strconv.Itoa(i))
		assert.NilError(t, os.WriteFile(path, []byte(c), 0644))
		paths = append(paths, path)
	}
	return paths
}

// writeTemplates writes content to each of paths, which are relative to the
// current dir, as the paths of the real templates are.
func writeTemplates(t *testing.T, paths []string, content string) {
	t.Helper()

	for _, path := range paths {
		assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0755))
		assert.NilError(t, os.WriteFile(path, []byte(content), 0644))
	}
}

// assertFiles checks that each of the files at paths holds want.
func assertFiles(t *testing.T, paths []string, want string) {
	t.Helper()

	for _, path := range paths {
		got, err := os.ReadFile(path)
		assert.NilError(t, err)
		assert.Equal(t, string(got), want, path)
	}
}

// serve starts a server that answers each path in bodies with its body,
// and any other path with 404 Not Found.
func serve(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A template that no longer matches once fails the bot's run, so check the
// real templates here: a template change that breaks a match, or adds a
// second one, fails its own PR.
func TestTemplatesMatch(t *testing.T) {
	t.Chdir("../../..")

	checkTemplates(t, pyprojectPaths, funcPythonVersionRe)
	checkPoms(t, quarkusPomPaths, quarkusVersionRe, 1)
	checkPoms(t, springBootPomPaths, springBootVersionRe, 2)
	checkPoms(t, springBootPomPaths, springCloudVersionRe, 2)
}

// checkTemplates checks that re matches once in each of the files at paths,
// with the same version in all, and returns it.
func checkTemplates(t *testing.T, paths []string, re *regexp.Regexp) string {
	t.Helper()

	version, err := readVersion(paths, re)
	assert.NilError(t, err, "%s", paths)
	return version
}

// checkPoms checks that re finds the same version in the poms at paths, and
// that updatePoms can work with it.
func checkPoms(t *testing.T, paths []string, re *regexp.Regexp, keep int) {
	t.Helper()

	version := checkTemplates(t, paths, re)
	_, err := newest(version, []string{version}, keep)
	assert.NilError(t, err, "%s in %s", version, paths)
}
