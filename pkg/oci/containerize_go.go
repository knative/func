package oci

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	slashpath "path"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	fn "knative.dev/func/pkg/functions"
)

// Build the source code as Go, cross compiled for the given platform, placing
// the statically linked binary in a tarred layer and return the Descriptor
// and Layer metadata.
func buildGoLayer(cfg *buildConfig, p v1.Platform) (desc v1.Descriptor, layer v1.Layer, err error) {
	// Executable
	exe, err := goBuild(cfg, p) // Compile binary returning its path
	if err != nil {
		return
	}

	// Tarball
	target := path(cfg.buildDir(), fmt.Sprintf("execlayer.%v.%v.tar.gz", p.OS, p.Architecture))
	if err = newExecTarball(exe, target, cfg.verbose); err != nil {
		return
	}

	// Layer
	if layer, err = tarball.LayerFromFile(target); err != nil {
		return
	}

	// Descriptor
	if desc, err = newDescriptor(layer); err != nil {
		return
	}
	desc.Platform = &p

	// Blob
	blob := path(cfg.blobsDir(), desc.Digest.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir(), target), rel(cfg.buildDir(), blob))
	}
	err = os.Rename(target, blob)
	return
}

func goBuild(cfg *buildConfig, p v1.Platform) (binPath string, err error) {
	gobin, args, outpath, err := goBuildCmd(p, cfg)
	if err != nil {
		return
	}
	envs := goBuildEnvs(cfg.f, p)
	if cfg.verbose {
		fmt.Printf("%v %v\n", gobin, strings.Join(args, " "))
	} else {
		fmt.Printf("   %v\n", filepath.Base(outpath))
	}

	// Build the function
	cmd := exec.CommandContext(cfg.ctx, gobin, args...)
	cmd.Env = envs
	cmd.Dir = cfg.buildDir()
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return outpath, cmd.Run()
}

func goBuildCmd(p v1.Platform, cfg *buildConfig) (gobin string, args []string, outpath string, err error) {
	/* TODO:  Use Build Command override from the function if provided
	 * A future PR will include the ability to specify a
	 * f.Build.BuildCommand, or BuildArgs for use here to customize
	 * This will be useful when, for example, the function is written in
	 *	Go and the function developer needs Libc compatibility, in which case
	 *	the default command will need to be replaced with:
	 *	go build -ldflags "-linkmode 'external' -extldflags '-static'"
	 *  Pseudocode:
	 *  if BuildArgs or BuildCommand
	 *    Validate command or args are safe to run
	 *      no other commands injected
	 *      does not contain Go's "toolexec"
	 *      does not specify the output path
	 *    Either replace or append to gobin
	 */

	// Use the binary specified FUNC_GO_PATH if defined
	gobin = os.Getenv("FUNC_GO_PATH") // TODO: move to main and plumb through
	if gobin == "" {
		gobin = "go"
	}

	// Build as ./func/builds/$PID/result/f.$OS.$Architecture
	name := fmt.Sprintf("f.%v.%v", p.OS, p.Architecture)
	if p.Variant != "" {
		name = name + "." + p.Variant
	}
	outpath = path(cfg.buildDir(), "result", name)
	args = []string{"build", "-o", outpath}
	return gobin, args, outpath, nil
}

func goBuildEnvs(f fn.Function, p v1.Platform) (envs []string) {
	pegged := []string{
		"CGO_ENABLED=0",
		"GOOS=" + p.OS,
		"GOARCH=" + p.Architecture,
	}
	if p.Variant != "" && p.Architecture == "arm" {
		pegged = append(pegged, "GOARM="+strings.TrimPrefix(p.Variant, "v"))
	} else if p.Variant != "" && p.Architecture == "amd64" {
		pegged = append(pegged, "GOAMD64="+p.Variant)
	}

	isPegged := func(env string) bool {
		for _, v := range pegged {
			name := strings.Split(v, "=")[0]
			if strings.HasPrefix(env, name) {
				return true
			}
		}
		return false
	}

	envs = append(envs, pegged...)
	for _, env := range os.Environ() {
		if !isPegged(env) {
			envs = append(envs, env)
		}
	}
	return envs
}

func newExecTarball(source, target string, verbose bool) error {
	targetFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	gw := gzip.NewWriter(targetFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}
	header.Mode = (header.Mode & ^int64(fs.ModePerm)) | 0755

	header.Name = slashpath.Join("/func", "f")
	// TODO: should we set file timestamps to the build start time of cfg.t?
	// header.ModTime = timestampArgument

	if err = tw.WriteHeader(header); err != nil {
		return err
	}
	if verbose {
		fmt.Printf("→ %v \n", header.Name)
	}

	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()

	i, err := io.Copy(tw, file)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Printf("  wrote %v bytes \n", i)
	}
	return nil
}
