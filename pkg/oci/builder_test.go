package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

var TestPlatforms = []fn.Platform{{OS: "linux", Architecture: runtime.GOARCH}}

// TestBuilder_BuildGo ensures that, when given a Go Function, an OCI-compliant
// directory structure is created on .Build in the expected path.
func TestBuilder_BuildGo(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New(fn.WithVerbose(true))

	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", true)

	if err := builder.Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	last := filepath.Join(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCIStructure(last, t) // validate OCI compliant
}

// TestBuilder_BuildPython ensures that, when given a Python Function, an
// OCI-compliant directory structure is created on .Build in the expected path.
func TestBuilder_BuildPython(t *testing.T) {
	testPython, _ := strconv.ParseBool(os.Getenv("FUNC_TEST_PYTHON"))
	if !testPython {
		// NOTE: language-specific tests will be integrated more wholistically
		// in our upcoming E2E test refactor
		t.Skip("Skipping test that requires special environment setup")
	}
	root, done := Mktemp(t)
	defer done()

	client := fn.New(fn.WithVerbose(true))

	f, err := client.Init(fn.Function{Root: root, Runtime: "python"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", true)

	if err := builder.Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	last := filepath.Join(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCIStructure(last, t) // validate OCI compliant
}

// TestBuilder_Files ensures that static files are added to the container
// image as expected.  This includes template files, regular files and links.
func TestBuilder_Files(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	// Create a function with the default template
	f, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	// Add a regular file
	if err := os.WriteFile("a.txt", []byte("file a"), 0644); err != nil {
		t.Fatal(err)
	}

	// Links
	var link struct {
		Target     string
		Mode       fs.FileMode
		Executable bool
	}
	if runtime.GOOS != "windows" {
		// Default case: use symlinks
		link.Target = "a.txt"
		link.Mode = fs.ModeSymlink
		link.Executable = true

		if err := os.Symlink("a.txt", "a.lnk"); err != nil {
			t.Fatal(err)
		}
	} else {
		// Windows: create a copy
		if err := os.WriteFile("a.lnk", []byte("file a"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := NewBuilder("", true).Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	expected := []fileInfo{
		{Path: "/etc/pki/tls/certs/ca-certificates.crt"},
		{Path: "/etc/ssl/certs/ca-certificates.crt"},
		{Path: "/func", Type: fs.ModeDir},
		{Path: "/func/README.md"},
		{Path: "/func/a.lnk", Linkname: link.Target, Type: link.Mode, Executable: link.Executable},
		{Path: "/func/a.txt"},
		{Path: "/func/f", Executable: true},
		{Path: "/func/func.yaml"},
		{Path: "/func/go.mod"},
		{Path: "/func/handle.go"},
		{Path: "/func/handle_test.go"},
	}

	last := filepath.Join(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCIFiles(last, expected, t)
}

// TestBuilder_Concurrency
func TestBuilder_Concurrency(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New()

	// Initialize a new Go Function
	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	// Concurrency
	//
	// The first builder is setup to use a mock implementation of the
	// builder function which will block until released after first notifying
	// that it has been paused.
	//
	// When the test receives the message that the builder has been paused, it
	// starts a second, concurrently executing builder to ensure there is a
	// typed error returned indicating a build is in progress.
	//
	// When the second builder completes, having confirmed the error message
	// received is as expected.  It signals the first (blocked) builder that it
	// can now continue.

	// Thet test waits until the first builder notifies that it is done, and
	// has therefore ran its tests as well.

	var (
		pausedCh   = make(chan bool)
		continueCh = make(chan bool)
		wg         sync.WaitGroup
	)

	// Build A
	builder1 := NewBuilder("builder1", true)
	testImplA := NewTestLanguageBuilder()
	testImplA.WritePlatformFn = func(job buildJob, p v1.Platform) ([]imageLayer, error) {
		if isFirstBuild(job, p) {
			pausedCh <- true // Notify of being paused
			<-continueCh     // Block until released
		}
		return []imageLayer{}, nil
	}
	builder1.impl = testImplA

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := builder1.Build(context.Background(), f, TestPlatforms); err != nil {
			t.Errorf("test build error: %v", err)
		}
	}()

	//  Wait until build 1 indicates it is paused
	<-pausedCh

	// Build B
	builder2 := NewBuilder("builder2", true)
	testImplB := NewTestLanguageBuilder()
	testImplB.WritePlatformFn = func(job buildJob, p v1.Platform) ([]imageLayer, error) {
		return []imageLayer{}, fmt.Errorf("the buildFn should not have been invoked")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = builder2.Build(context.Background(), f, TestPlatforms)
		if !errors.As(err, &ErrBuildInProgress{}) {
			t.Errorf("test build error: %v", err)
		}
	}()

	// Release the blocking Build A and wait until complete.
	continueCh <- true
	wg.Wait()
}

func isFirstBuild(cfg buildJob, current v1.Platform) bool {
	first := cfg.platforms[0]
	return current.OS == first.OS &&
		current.Architecture == first.Architecture &&
		current.Variant == first.Variant
}

// ImageIndex represents the structure of an OCI Image Index.
type ImageIndex struct {
	SchemaVersion int `json:"schemaVersion"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
}

// validateOCIStructue performs a cursory check that the given path exists and
// has the basics of an OCI compliant structure.
func validateOCIStructure(path string, t *testing.T) {
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unable to stat output path. %v", path)
		return
	}

	ociLayoutFile := filepath.Join(path, "oci-layout")
	indexJSONFile := filepath.Join(path, "index.json")
	blobsDir := filepath.Join(path, "blobs")

	// Check if required files and directories exist
	if _, err := os.Stat(ociLayoutFile); os.IsNotExist(err) {
		t.Fatal("missing oci-layout file")
	}
	if _, err := os.Stat(indexJSONFile); os.IsNotExist(err) {
		t.Fatal("missing index.json file")
	}
	if _, err := os.Stat(blobsDir); os.IsNotExist(err) {
		t.Fatal("missing blobs directory")
	}

	// Load and validate index.json
	indexJSONData, err := os.ReadFile(indexJSONFile)
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}

	var imageIndex ImageIndex
	err = json.Unmarshal(indexJSONData, &imageIndex)
	if err != nil {
		t.Fatalf("failed to parse index.json: %v", err)
	}

	if imageIndex.SchemaVersion != 2 {
		t.Fatalf("invalid schema version, expected 2, got %d", imageIndex.SchemaVersion)
	}

	if len(imageIndex.Manifests) < 1 {
		t.Fatal("no manifests")
	}
}

// validateOCIFiles ensures that the OCI image at path contains files with
// the given attributes.
func validateOCIFiles(path string, expected []fileInfo, t *testing.T) {
	// Load the Image Index
	bb, err := os.ReadFile(filepath.Join(path, "index.json"))
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}
	var imageIndex ImageIndex
	if err = json.Unmarshal(bb, &imageIndex); err != nil {
		t.Fatalf("failed to parse index.json: %v", err)
	}

	// Load the first manifest
	digest := strings.TrimPrefix(imageIndex.Manifests[0].Digest, "sha256:")
	manifestFile := filepath.Join(path, "blobs", "sha256", digest)
	manifestFileData, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	mf := struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}{}
	err = json.Unmarshal(manifestFileData, &mf)
	if err != nil {
		t.Fatal(err)
	}
	var files []fileInfo

	for _, layer := range mf.Layers {
		func() {
			digest = strings.TrimPrefix(layer.Digest, "sha256:")
			f, err := os.Open(filepath.Join(path, "blobs", "sha256", digest))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			gr, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			defer gr.Close()

			tr := tar.NewReader(gr)
			for {
				hdr, err := tr.Next()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatal(err)
				}
				files = append(files, fileInfo{
					Path:       hdr.Name,
					Type:       hdr.FileInfo().Mode() & fs.ModeType,
					Executable: (hdr.FileInfo().Mode()&0111 == 0111) && !hdr.FileInfo().IsDir(),
					Linkname:   hdr.Linkname,
				})
			}
		}()
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	if diff := cmp.Diff(expected, files); diff != "" {
		t.Error("files in oci differ from expectation (-want, +got):", diff)
	}
}

type fileInfo struct {
	Path       string
	Type       fs.FileMode
	Executable bool
	Linkname   string
}

// TestBuilder_StaticEnvs ensures that certain "static" environment variables
// comprising Function metadata are added to the config.
func TestBuilder_StaticEnvs(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	staticEnvs := []string{
		"FUNC_CREATED",
		"FUNC_VERSION",
	}

	f, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	if err := NewBuilder("", true).Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	// Assert
	// Check if the OCI container defines at least one of the static
	// variables on each of the constituent containers.
	// ---
	// Get the images list (manifest descripors) from the index
	ociPath := filepath.Join(f.Root, fn.RunDataDir, "builds", "last", "oci")
	data, err := os.ReadFile(filepath.Join(ociPath, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	for _, manifestDesc := range index.Manifests {

		// Dereference the manifest descriptor into the referenced image manifest
		manifestHash := strings.TrimPrefix(manifestDesc.Digest, "sha256:")
		data, err := os.ReadFile(filepath.Join(ociPath, "blobs", "sha256", manifestHash))
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}

		// From the image manifest get the image's config.json
		configHash := strings.TrimPrefix(manifest.Config.Digest, "sha256:")
		data, err = os.ReadFile(filepath.Join(ociPath, "blobs", "sha256", configHash))
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Config struct {
				Env []string `json:"Env"`
			} `json:"config"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			panic(err)
		}

		containsEnv := func(ss []string, name string) bool {
			for _, s := range ss {
				if strings.HasPrefix(s, name) {
					return true
				}
			}
			return false
		}

		for _, expected := range staticEnvs {
			t.Logf("checking for %q in slice %v", expected, config.Config.Env)
			if containsEnv(config.Config.Env, expected) {
				continue // to check the rest
			}
			t.Fatalf("static env %q not found in resultant container", expected)
		}
	}
}

// -----------  Mock Language Builder Impl ------

// TestLanguageBuilder is the language-specific builder implementation used by the
// OCI builder for each language, and can be overridden for testing
type TestLanguageBuilder struct {
	BaseInvoked bool
	BaseFn      func() string

	WriteSharedInvoked bool
	WriteSharedFn      func(buildJob) ([]imageLayer, error)

	WritePlatformInvoked bool
	WritePlatformFn      func(buildJob, v1.Platform) ([]imageLayer, error)

	ConfigureInvoked bool
	ConfigureFn      func(buildJob, v1.Platform, v1.ConfigFile) (v1.ConfigFile, error)
}

func NewTestLanguageBuilder() *TestLanguageBuilder {
	return &TestLanguageBuilder{
		BaseFn:          func() string { return "" },
		WriteSharedFn:   func(buildJob) ([]imageLayer, error) { return []imageLayer{}, nil },
		WritePlatformFn: func(buildJob, v1.Platform) ([]imageLayer, error) { return []imageLayer{}, nil },
		ConfigureFn: func(buildJob, v1.Platform, v1.ConfigFile) (v1.ConfigFile, error) {
			return v1.ConfigFile{}, nil
		},
	}
}

func (l *TestLanguageBuilder) Base() string {
	l.BaseInvoked = true
	return l.BaseFn()
}

func (l *TestLanguageBuilder) WriteShared(job buildJob) ([]imageLayer, error) {
	l.WriteSharedInvoked = true
	return l.WriteSharedFn(job)
}

func (l *TestLanguageBuilder) WritePlatform(job buildJob, p v1.Platform) ([]imageLayer, error) {
	l.WritePlatformInvoked = true
	return l.WritePlatformFn(job, p)
}

func (l *TestLanguageBuilder) Configure(job buildJob, p v1.Platform, c v1.ConfigFile) (v1.ConfigFile, error) {
	l.ConfigureInvoked = true
	return l.ConfigureFn(job, p, c)
}

// Test_validatedLinkTaarget ensures that the function disallows
// links which are absolute or refer to targets outside the given root, in
// addition to the basic job of returning the value of reading the link.
func Test_validatedLinkTarget(t *testing.T) {
	root := filepath.Join("testdata", "test-links")

	err := os.Symlink("/var/example/absolute/link", filepath.Join(root, "absoluteLink"))
	if err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	err = os.Symlink("c://some/absolute/path", filepath.Join(root, "absoluteLinkWindows"))
	if err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}

	// Windows-specific absolute link and link target values:
	absoluteLink := "absoluteLink"
	linkTarget := "./a.txt"
	if runtime.GOOS == "windows" {
		absoluteLink = "absoluteLinkWindows"
		linkTarget = ".\\a.txt"
	}

	tests := []struct {
		path   string // path of the file within test project root
		valid  bool   // If it should be considered valid
		target string // optional test of the returned value (target)
		name   string // descriptive name of the test
	}{
		{absoluteLink, false, "", "disallow absolute-path links on linux"},
		{"a.lnk", true, linkTarget, "spot-check link target"},
		{"a.lnk", true, "", "links to files within the root are allowed"},
		{"...validName.lnk", true, "", "allow links with target of dot prefixed names"},
		{"linkToRoot", true, "", "allow links to the project root"},
		{"b/linkToRoot", true, "", "allow links to the project root from within subdir"},
		{"b/linkToCurrentDir", true, "", "allow links to a subdirectory within the project"},
		{"b/linkToRootsParent", false, "", "disallow links to the project's immediate parent"},
		{"b/linkOutsideRootsParent", false, "", "disallow links outside project root and its parent"},
		{"b/c/linkToParent", true, "", " allow links up, but within project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(root, tt.path)
			target, err := validatedLinkTarget(root, path)

			if err == nil != tt.valid {
				t.Fatalf("expected validity '%v', got '%v'", tt.valid, err)
			}
			if tt.target != "" && target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}

}
