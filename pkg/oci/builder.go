package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	fn "knative.dev/func/pkg/functions"
)

var path = filepath.Join

// TODO: This may no longer be necessary, delete if e2e and acceptance tests
// succeed:
// const DefaultName = builders.Host

var defaultPlatforms = []v1.Platform{
	{OS: "linux", Architecture: "amd64"},
	{OS: "linux", Architecture: "arm64"},
	{OS: "linux", Architecture: "arm", Variant: "v7"},
}

var defaultIgnored = []string{ // TODO: implement and use .funcignore
	".git",
	".func",
	".funcignore",
	".gitignore",
}

// BuildErr indicates a build error occurred.
type BuildErr struct {
	Err error
}

func (e BuildErr) Error() string {
	return fmt.Sprintf("error performing host build. %v", e.Err)
}

// Builder which creates an OCI-compliant multi-arch (index) container from
// the function at path.
type Builder struct {
	name    string
	verbose bool
}

// NewBuilder creates a builder instance.
func NewBuilder(name string, verbose bool) *Builder {
	return &Builder{name, verbose}
}

// Build an OCI-compliant Mult-arch (v1.ImageIndex) container on disk
// in the function's runtime data directory at:
//
//	.func/builds/by-hash/$HASH
//
// Updates a symlink to this directory at:
//
//	.func/builds/last
func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	cfg := &buildConfig{ctx, f, time.Now(), b.verbose, ""}

	if err = setup(cfg); err != nil { // create directories and links
		return
	}
	defer teardown(cfg)

	//TODO: Use scaffold package when merged:
	/*
		if err = scaffolding.Scaffold(ctx, f, cfg.buildDir()); err != nil {
			return
		}
	*/
	// IN the meantime, use an airball mainfile
	data := `
package main

import "fmt"

func main () {
  fmt.Println("Hello, world!")
}
`
	if err = os.WriteFile(path(cfg.buildDir(), "main.go"), []byte(data), 0664); err != nil {
		return
	}

	if err = containerize(cfg); err != nil {
		return
	}
	return updateLastLink(cfg)

	// TODO: communicating build completeness throgh returning without error
	// relies on the implicit availability of the OIC image in this process'
	// build directory.  Would be better to have a formal build result object
	// which includes a general struct which can be used by all builders to
	// communicate to the pusher where the image can be found.
}

// buildConfig contains various settings for a single build
type buildConfig struct {
	ctx     context.Context // build context
	f       fn.Function     // Function being built
	t       time.Time       // Timestamp for this build
	verbose bool            // verbose logging
	h       string          // hash cache (use .hash() accessor)
}

func (c *buildConfig) hash() string {
	if c.h != "" {
		return c.h
	}
	var err error
	if c.h, _, err = fn.Fingerprint(c.f.Root); err != nil {
		fmt.Fprintf(os.Stderr, "error calculating fingerprint for build. %v", err)
	}
	return c.h
}

func (c *buildConfig) lastLink() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "last")
}
func (c *buildConfig) pidsDir() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-pid")
}
func (c *buildConfig) pidLink() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-pid", strconv.Itoa(os.Getpid()))
}
func (c *buildConfig) buildsDir() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-hash")
}
func (c *buildConfig) buildDir() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-hash", c.hash())
}
func (c *buildConfig) ociDir() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-hash", c.hash(), "oci")
}
func (c *buildConfig) blobsDir() string {
	return path(c.f.Root, fn.RunDataDir, "builds", "by-hash", c.hash(), "oci", "blobs", "sha256")
}

// setup errors if there already exists a build directory.  Otherwise, it
// creates a build directory based on the function's hash, and creates
// a link to this build directory for the current pid to denote the build
// is in progress.
func setup(cfg *buildConfig) (err error) {
	// error if already in progress
	if isActive(cfg, cfg.buildDir()) {
		return BuildErr{fmt.Errorf("Build directory already exists for this version hash and is associated with an active PID.  Is a build already in progress? %v", cfg.buildDir())}
	}

	// create build files directory
	if cfg.verbose {
		fmt.Printf("mkdir -p %v\n", cfg.buildDir())
	}
	if err = os.MkdirAll(cfg.buildDir(), 0774); err != nil {
		return
	}

	// create pid links directory
	if _, err = os.Stat(cfg.pidsDir()); os.IsNotExist(err) {
		if cfg.verbose {
			fmt.Printf("mkdir -p %v\n", cfg.pidsDir())
		}
		if err = os.MkdirAll(cfg.pidsDir(), 0774); err != nil {
			return
		}
	}

	// create a link named $pid to the current build files directory
	target := path("..", "by-hash", cfg.hash())
	if cfg.verbose {
		fmt.Printf("ln -s %v %v\n", target, cfg.pidLink())
	}
	return os.Symlink(target, cfg.pidLink())
}

func teardown(cfg *buildConfig) {
	// remove the pid link for the current process indicating the build is
	// no longer in progress.
	_ = os.RemoveAll(cfg.pidLink())

	// remove pid links for processes which no longer exist.
	dd, _ := os.ReadDir(cfg.pidsDir())
	for _, d := range dd {
		if processExists(d.Name()) {
			continue
		}
		dir := path(cfg.pidsDir(), d.Name())
		if cfg.verbose {
			fmt.Printf("rm %v\n", dir)
		}
		_ = os.RemoveAll(dir)
	}

	// remove build file directories unless they are either:
	// 1. The build files from the last successful build
	// 2. Are associated with a pid link (currently in progress)
	dd, _ = os.ReadDir(cfg.buildsDir())
	for _, d := range dd {
		dir := path(cfg.buildsDir(), d.Name())
		if isLinkTo(cfg.lastLink(), dir) {
			continue
		}
		if isActive(cfg, dir) {
			continue
		}
		if cfg.verbose {
			fmt.Printf("rm %v\n", dir)
		}
		_ = os.RemoveAll(dir)
	}
}

func processExists(pid string) bool {
	p, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	process, err := os.FindProcess(p)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func isLinkTo(link, target string) bool {
	var err error
	if link, err = filepath.EvalSymlinks(link); err != nil {
		return false
	}
	if link, err = filepath.Abs(link); err != nil {
		return false
	}

	if target, err = filepath.EvalSymlinks(target); err != nil {
		return false
	}
	if target, err = filepath.Abs(target); err != nil {
		return false
	}

	return link == target
}

// isActive returns whether or not the given directory path is for a build
// which is currently active or in progress.
func isActive(cfg *buildConfig, dir string) bool {
	dd, _ := os.ReadDir(cfg.pidsDir())
	for _, d := range dd {
		link := path(cfg.pidsDir(), d.Name())
		if processExists(d.Name()) && isLinkTo(link, dir) {
			return true
		}
	}
	return false
}

func updateLastLink(cfg *buildConfig) error {
	if cfg.verbose {
		fmt.Printf("ln -s %v %v\n", cfg.buildDir(), cfg.lastLink())
	}
	_ = os.RemoveAll(cfg.lastLink())
	return os.Symlink(cfg.buildDir(), cfg.lastLink())
}
