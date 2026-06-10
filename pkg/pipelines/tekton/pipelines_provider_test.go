package tekton

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/func/pkg/functions"
)

func TestSourcesAsTarStream(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("**/a.out"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("Hello World!\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hello.txt", filepath.Join(root, "hello.txt.lnk")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	rc := sourcesAsTarStream(fn.Function{Root: root})
	t.Cleanup(func() { _ = rc.Close() })

	var helloTxtContent []byte
	var symlinkFound bool
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if hdr.Name == "source/bin/release/a.out" {
			t.Error("stream contains file that should have been ignored")
		}
		if strings.HasPrefix(hdr.Name, "source/.git") {
			t.Errorf(".git files were included: %q", hdr.Name)
		}
		if hdr.Name == "source/hello.txt" {
			helloTxtContent, err = io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
		}
		if hdr.Name == "source/hello.txt.lnk" {
			symlinkFound = true
			if hdr.Linkname != "hello.txt" {
				t.Errorf("bad symlink target: %q", hdr.Linkname)
			}
		}
	}
	if helloTxtContent == nil {
		t.Error("the hello.txt file is missing in the stream")
	} else {
		if string(helloTxtContent) != "Hello World!\n" {
			t.Error("the hello.txt file has incorrect content")
		}
	}
	if !symlinkFound {
		t.Error("symlink missing in the stream")
	}
}

// TestSourcesAsTarStream_ExcludesFuncDir ensures the local runtime directory
// (.func) is never uploaded to the cluster — most importantly the credential
// file .func/local.yaml — even when the function has NO .gitignore. The
// exclusion must come from the authoritative DefaultIgnored policy, not from a
// .gitignore happening to list it. Scaffolding under .func/build is regenerated
// on-cluster by the func-scaffold step, so it is not needed in the upload.
func TestSourcesAsTarStream_ExcludesFuncDir(t *testing.T) {
	root := t.TempDir()
	// deliberately NO .gitignore is written
	mustWrite := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("app.js", "module.exports = () => {}\n") // a normal source file
	mustWrite(".func/local.yaml", "auth:\n- cluster-url: https://secret\n  user:\n    token: SECRET\n")
	mustWrite(".func/build/service/main.py", "# scaffolding\n")
	mustWrite(".func/built-image", "example.com/img@sha256:deadbeef\n")

	rc := sourcesAsTarStream(fn.Function{Root: root})
	t.Cleanup(func() { _ = rc.Close() })

	var appFound bool
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if strings.HasPrefix(hdr.Name, "source/.func") {
			t.Errorf(".func entry leaked into the upload stream: %q", hdr.Name)
		}
		if hdr.Name == "source/app.js" {
			appFound = true
		}
	}
	if !appFound {
		t.Error("the app.js source file is missing from the stream")
	}
}

func Test_createPipelinePersistentVolumeClaim(t *testing.T) {
	type mockType func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error)

	type args struct {
		ctx       context.Context
		f         fn.Function
		namespace string
		labels    map[string]string
		size      string
	}
	tests := []struct {
		name    string
		args    args
		mock    mockType
		wantErr bool
	}{
		{
			name: "returns error if pvc creation failed",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return errors.New("creation of pvc failed")
			},
			wantErr: true,
		},
		{
			name: "returns nil if pvc already exists",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return &apiErrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonAlreadyExists}}
			},
			wantErr: false,
		},
		{
			name: "returns err if namespace not defined and default returns an err",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return errors.New("no namespace defined")
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { // save current function and restore it at the end
			old := createPersistentVolumeClaim
			defer func() { createPersistentVolumeClaim = old }()

			createPersistentVolumeClaim = tt.mock
			tt.args.f.Build.PVCSize = tt.args.size
			if err := createPipelinePersistentVolumeClaim(tt.args.ctx, tt.args.f, tt.args.namespace, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("createPipelinePersistentVolumeClaim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSourcesAsTarStream_InjectsEffectiveConfig: the uploaded func.yaml is the
// serialized in-memory (effective) config, not the on-disk copy, and the
// on-disk file is left untouched.
func TestSourcesAsTarStream_InjectsEffectiveConfig(t *testing.T) {
	root := t.TempDir()
	diskYaml := "specVersion: 0.36.0\nname: my-fn\nruntime: go\ncreated: 2026-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(root, "func.yaml"), []byte(diskYaml), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// Effective, not-yet-disk-persisted configuration, as set by deploy flags.
	f.Namespace = "effective-ns"
	f.Deploy.ServiceAccountName = "effective-sa"

	rc := sourcesAsTarStream(f)
	t.Cleanup(func() { _ = rc.Close() })

	var uploaded []byte
	entries := 0
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if hdr.Name == "source/func.yaml" {
			entries++
			if uploaded, err = io.ReadAll(tr); err != nil {
				t.Fatal(err)
			}
		}
	}
	if entries != 1 {
		t.Fatalf("expected exactly one func.yaml entry in the stream, got %d", entries)
	}

	want, err := f.MarshalFuncYaml()
	if err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != string(want) {
		t.Errorf("uploaded func.yaml != f.MarshalFuncYaml()\n--- uploaded ---\n%s\n--- want ---\n%s", uploaded, want)
	}

	extract := t.TempDir()
	if err := os.WriteFile(filepath.Join(extract, "func.yaml"), uploaded, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := fn.NewFunction(extract)
	if err != nil {
		t.Fatalf("uploaded func.yaml does not parse: %v", err)
	}
	if got.Namespace != "effective-ns" || got.Deploy.ServiceAccountName != "effective-sa" {
		t.Errorf("parsed uploaded func.yaml lost effective values: ns=%q sa=%q",
			got.Namespace, got.Deploy.ServiceAccountName)
	}

	// The on-disk func.yaml must remain untouched.
	disk, err := os.ReadFile(filepath.Join(root, "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != diskYaml {
		t.Errorf("on-disk func.yaml was mutated by the upload:\n%s", disk)
	}
}
