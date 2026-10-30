// Package main updates the middleware versions in the function templates to
// their newest compatible releases. Middleware is what
// pkg/scaffolding/middleware_version.go detects for each runtime:
//
//   - python: func-python moves to the release that PyPI calls its latest.
//   - quarkus: the Quarkus platform moves to the newest final release of its
//     current major version.
//   - springboot: Spring Boot moves to the newest patch of its current minor
//     version, and Spring Cloud to the newest patch of its current release
//     train.
//
// We maintain func-python ourselves, so it moves to new major versions too. A
// new Spring Boot minor needs a matching Spring Cloud train, and a new major
// version of Quarkus or Spring Boot may need the templates ported, so a person
// makes those moves. Node and TypeScript (faas-js-runtime) are left out
// because their package-lock.json files need npm to update.
//
// Every middleware is tried, even after one fails, and the tool then exits 1
// if any failed. That fails the dependency bot's run, which alerts us, so a
// broken source does not stop the bumps unseen.
//
// Run it from the repository root, usually via "make update-templates",
// which hack/update-deps.sh calls when the dependency bot runs it.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

// A middleware is what the templates of one runtime build on, and how to
// update it.
type middleware struct {
	runtime string
	name    string
	update  func(context.Context) error
}

// middlewares returns the middlewares, each updated from the servers in src.
func middlewares(src sources) []middleware {
	return []middleware{
		{"python", "func-python", src.updatePython},
		{"quarkus", "Quarkus platform", src.updateQuarkus},
		{"springboot", "Spring Boot and Spring Cloud", src.updateSpringBoot},
	}
}

// sources are the servers the tool asks about releases. Tests point them at
// fake servers.
type sources struct {
	// maven is a Maven repository, for Spring Boot and Spring Cloud.
	maven string
	// pypi is the PyPI JSON API, for func-python.
	pypi string
	// quarkus is the code.quarkus.io platforms API, for Quarkus.
	quarkus string
}

// defaultSources are the real servers.
var defaultSources = sources{
	maven:   "https://repo1.maven.org/maven2",
	pypi:    "https://pypi.org/pypi",
	quarkus: "https://code.quarkus.io/api/platforms",
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(130)
	}()

	if err := run(ctx, middlewares(defaultSources)); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// run updates each of middlewares in turn. It tries them all, even after one
// fails, so one run reports every broken source.
func run(ctx context.Context, middlewares []middleware) error {
	var failed []string
	for _, m := range middlewares {
		fmt.Printf("Updating %s middleware (%s)...\n", m.runtime, m.name)
		updateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err := m.update(updateCtx)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s middleware: %v\n", m.runtime, err)
			failed = append(failed, m.runtime)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed middleware updates: %s", strings.Join(failed, ", "))
	}
	return nil
}

// updateFiles sets the version that re captures in the files at paths to
// version. The files must hold the same version, which is checked before any
// file is written.
func updateFiles(paths []string, re *regexp.Regexp, version string) error {
	current, err := readVersion(paths, re)
	if err != nil {
		return err
	}
	if version == current {
		fmt.Printf("  %s is up-to-date\n", current)
		return nil
	}

	for _, path := range paths {
		if err := writeVersion(path, re, version); err != nil {
			return err
		}
		fmt.Printf("  %s: updated %s to %s\n", path, current, version)
	}
	return nil
}

// readVersion returns the version that re captures in the files at paths.
// re must match once in each file, and the files must all hold the same
// version.
func readVersion(paths []string, re *regexp.Regexp) (string, error) {
	var version string
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		matches := re.FindAllSubmatch(data, -1)
		if len(matches) == 0 {
			return "", fmt.Errorf("%s: no match for %s", path, re)
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("%s: %d matches for %s, want 1", path, len(matches), re)
		}
		m := matches[0]
		if i > 0 && string(m[1]) != version {
			return "", fmt.Errorf("%s has %s, but %s has %s", paths[0], version, path, m[1])
		}
		version = string(m[1])
	}
	return version, nil
}

// writeVersion sets the version that re captures in the file at path.
func writeVersion(path string, re *regexp.Regexp, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m := re.FindSubmatchIndex(data)
	if m == nil {
		return fmt.Errorf("%s: no match for %s", path, re)
	}
	data = slices.Concat(data[:m[2]], []byte(version), data[m[3]:])
	return os.WriteFile(path, data, 0644)
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
