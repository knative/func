package oci

import (
	// "fmt"
	// "os"
	// "os/exec"
	// 	"path/filepath"

	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	slashpath "path"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	// "github.com/google/go-containerregistry/pkg/v1/tarball"
)

// var defaultPythonBase = "registry.access.redhat.com/ubi9/python-312:latest"
var defaultPythonBase = "python:3.13-slim"

type pythonBuilder struct{}

func (b pythonBuilder) Base() string {
	return defaultPythonBase
}

func (b pythonBuilder) Configure(job buildJob, _ v1.Platform, cf v1.ConfigFile) (v1.ConfigFile, error) {
	var (
		svcRelPath, _ = filepath.Rel(job.function.Root, job.buildDir()) // eg .func/builds/by-hash/$HASH
		svcPath       = filepath.Join("/func", svcRelPath)              // eg /func/.func/builds/by-hash/$HASH
		pythonPathEnv = fmt.Sprintf("PYTHONPATH=%v/lib", svcPath)
		mainPath      = fmt.Sprintf("%v/service/main.py", svcPath)
	)

	cf.Config.Env = append(cf.Config.Env, pythonPathEnv)
	cf.Config.Cmd = []string{"python", mainPath}
	return cf, nil
}

func (b pythonBuilder) WriteShared(job buildJob) (layers []imageLayer, err error) {
	var desc v1.Descriptor
	var layer v1.Layer

	// TODO:
	// Create a .func/bin which is used for transient binaries and install
	// pip there using https://bootstrap.pypa.io/get-pip.py if it either
	// does not exist or it reports version less than "const minPipVersion"
	// This will allow us to have the only toolchain dependency being
	// python itself.

	// Install Dependencies of the current project into ./lib
	// In the scaffolding direcotory.
	if job.verbose {
		fmt.Printf("pip install --target .\n")
	}
	cmd := exec.CommandContext(job.ctx, "pip", "install",
		"--target", filepath.Join(job.buildDir(), "lib"),
		filepath.Join(job.buildDir()))
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err = cmd.Run(); err != nil {
		return
	}

	// Tar up the now-final build directory
	source := job.buildDir()
	target := filepath.Join(job.buildDir(), "lib.tar.gz")
	if err = newPythonLibTarball(job, source, target); err != nil {
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

	// Blob
	blob := filepath.Join(job.blobsDir(), desc.Digest.Hex)
	if job.verbose {
		fmt.Printf("mv %v %v\n", rel(job.buildDir(), target), rel(job.buildDir(), blob))
	}
	err = os.Rename(target, blob)

	return []imageLayer{{Descriptor: desc, Layer: layer}}, nil
}

func newPythonLibTarball(job buildJob, root, target string) error {
	// Create a tarball of the "build directory"
	// when extracted, it's root will be /func
	// all files within should have path prefix .func/builds/by-hash/$hash

	targetFile, err := os.Create(target) // final .tar.gz
	if err != nil {
		return err
	}
	defer targetFile.Close()

	gw := gzip.NewWriter(targetFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// FIXME:
		// This is not ideal because we have to explicitly ignore the
		// oci directory from being tarred and the tar from itself.  In
		// hindsight, it would have been better to have the "build" directory
		// contain two sub-directories
		// ./dist  -  the scaffolding, libraries and link to the source code.
		// ./container  -  the final OCI container.
		if path == job.ociDir() {
			return filepath.SkipDir
		}
		if path == target {
			return nil
		}

		lnk := "" // if link, this will be used as the target
		if info.Mode()&fs.ModeSymlink != 0 {
			if lnk, err = validatedLinkTarget(job.function.Root, path); err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, lnk)
		if err != nil {
			return err
		}

		// The relative path from the function's root to the file
		relPath, err := filepath.Rel(job.function.Root, path)
		if err != nil {
			return err
		}
		header.Name = slashpath.Join("/func/", filepath.ToSlash(relPath))
		header.Uid = DefaultUid
		header.Gid = DefaultGid
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() { //nothing more to do for non-regular
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}

func (b pythonBuilder) WritePlatform(ctx buildJob, p v1.Platform) (layers []imageLayer, err error) {
	return []imageLayer{}, nil
}
