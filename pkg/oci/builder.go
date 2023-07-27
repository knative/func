package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/scaffolding"
)

var path = filepath.Join

var defaultIgnored = []string{ // TODO: implement and use .funcignore
	".git",
	".func",
	".funcignore",
	".gitignore",
}

// Builder which creates an OCI-compliant multi-arch (index) container from
// the function at path.
type Builder struct {
	name    string
	verbose bool

	onDone  func()               // optionally provide a function to be notified on done
	buildFn languageLayerBuilder // optionally provide a custom build impl
}

// NewBuilder creates a builder instance.
func NewBuilder(name string, verbose bool) *Builder {
	return &Builder{name, verbose, nil, nil}
}

func newBuildConfig(ctx context.Context, b *Builder, f fn.Function, platforms []fn.Platform) *buildConfig {
	c := &buildConfig{
		ctx,
		b.name,
		f,
		time.Now(),
		b.verbose,
		"",
		toPlatforms(platforms),
		b.onDone,
		b.buildFn,
	}
	// If the client did not specifically request a certain set of platforms,
	// use the func core defined set of suggested defaults.
	if len(platforms) == 0 {
		c.platforms = toPlatforms(fn.DefaultPlatforms)
	}
	return c
}

// Build an OCI-compliant Mult-arch (v1.ImageIndex) container on disk
// in the function's runtime data directory at:
//
//	.func/builds/by-hash/$HASH
//
// Updates a symlink to this directory at:
//
//	.func/builds/last
func (b *Builder) Build(ctx context.Context, f fn.Function, pp []fn.Platform) (err error) {
	cfg := newBuildConfig(ctx, b, f, pp)

	if err = setup(cfg); err != nil {
		return
	}
	defer teardown(cfg)

	// Load the embedded repository
	repo, err := fn.NewRepository("", "")
	if err != nil {
		return
	}

	// Write out the scaffolding
	err = scaffolding.Write(cfg.buildDir(), f.Root, f.Runtime, f.Invoke, repo.FS())
	if err != nil {
		return
	}

	// Create an OCI container from the scaffolded function
	if err = containerize(cfg); err != nil {
		return
	}

	if err = updateLastLink(cfg); err != nil {
		return
	}

	// TODO: communicating build completeness throgh returning without error
	// relies on the implicit availability of the OIC image in this process'
	// build directory.  Would be better to have a formal build result object
	// which includes a general struct which can be used by all builders to
	// communicate to the pusher where the image can be found.
	// Tests, however, can use a simple channel:
	if cfg.onDone != nil {
		cfg.onDone()
	}
	return
}

// buildConfig contains various settings for a single build
type buildConfig struct {
	ctx       context.Context // build context
	name      string
	f         fn.Function // Function being built
	t         time.Time   // Timestamp for this build
	verbose   bool        // verbose logging
	h         string      // hash cache (use .hash() accessor)
	platforms []v1.Platform
	onDone    func()               // optionally provide a function to be notified on done
	buildFn   languageLayerBuilder // optionally provide a custom build impl
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

func setup(cfg *buildConfig) (err error) {
	if isActive(cfg, cfg.buildDir()) {
		return ErrBuildInProgress{cfg.buildDir()}
	}

	// create build directory, recreating if it already existed
	if _, err = os.Stat(cfg.buildDir()); !os.IsNotExist(err) {
		if cfg.verbose {
			fmt.Printf("rm -rf %v\n", cfg.buildDir())
		}
		if err = os.RemoveAll(cfg.buildDir()); err != nil {
			return
		}
	}
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
	if runtime.GOOS == "windows" {
		return true
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
	rp, err := filepath.Rel(filepath.Dir(cfg.lastLink()), cfg.buildDir())
	if err != nil {
		return err
	}
	return os.Symlink(rp, cfg.lastLink())
}

// toPlatforms converts func's implementation-agnostic Platform struct
// into to the OCI builder's implementation-specific go-containerregistry v1
// palatform.
// Examples:
// {OS: "linux", Architecture: "amd64"},
// {OS: "linux", Architecture: "arm64"},
// {OS: "linux", Architecture: "arm", Variant: "v6"},
// {OS: "linux", Architecture: "arm", Variant: "v7"},
// {OS: "darwin", Architecture: "amd64"},
// {OS: "darwin", Architecture: "arm64"},
func toPlatforms(pp []fn.Platform) []v1.Platform {
	platforms := make([]v1.Platform, len(pp))
	for i, p := range pp {
		platforms[i] = v1.Platform{OS: p.OS, Architecture: p.Architecture, Variant: p.Variant}
	}
	return platforms
}
