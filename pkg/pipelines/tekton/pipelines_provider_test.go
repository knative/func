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

// readFuncYamlFromStream returns the content of source/func.yaml from the
// tar stream, or fails if it is absent.
func readFuncYamlFromStream(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if hdr.Name == "source/"+fn.FunctionFile {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatalf("source/%s missing from tar stream", fn.FunctionFile)
	return nil
}

// TestSourcesAsTarStream_FuncYamlInjection reproduces the fix for issue
// #3679: the func.yaml placed into the upload tar stream must reflect the
// in-memory function (with CLI overrides applied), NOT the stale on-disk
// file, and streaming must not mutate the on-disk file.
func TestSourcesAsTarStream_FuncYamlInjection(t *testing.T) {
	t.Run("overrides on-disk func.yaml", func(t *testing.T) {
		root := t.TempDir()

		// Stale on-disk func.yaml (as if from a previous deploy).
		onDisk := fn.Function{Root: root, Runtime: "go", Name: "fn"}
		onDisk.Deploy.ImagePullSecret = "stale-on-disk"
		if err := onDisk.Write(); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(filepath.Join(root, fn.FunctionFile))
		if err != nil {
			t.Fatal(err)
		}

		// In-memory function with a one-off CLI override.
		f := fn.Function{Root: root, Runtime: "go", Name: "fn"}
		f.Deploy.ImagePullSecret = "from-cli"

		rc := sourcesAsTarStream(f)
		t.Cleanup(func() { _ = rc.Close() })

		streamed := readFuncYamlFromStream(t, rc)
		if !strings.Contains(string(streamed), "imagePullSecret: from-cli") {
			t.Fatalf("tar func.yaml did not reflect in-memory override; got:\n%s", streamed)
		}
		if strings.Contains(string(streamed), "stale-on-disk") {
			t.Fatalf("tar func.yaml contains stale on-disk value; got:\n%s", streamed)
		}

		// The on-disk file must be untouched by streaming.
		after, err := os.ReadFile(filepath.Join(root, fn.FunctionFile))
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("sourcesAsTarStream mutated on-disk func.yaml.\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("injects when func.yaml absent on disk", func(t *testing.T) {
		root := t.TempDir()
		f := fn.Function{Root: root, Runtime: "go", Name: "fn"}
		f.Deploy.ImagePullSecret = "from-cli"

		rc := sourcesAsTarStream(f)
		t.Cleanup(func() { _ = rc.Close() })

		streamed := readFuncYamlFromStream(t, rc)
		if !strings.Contains(string(streamed), "imagePullSecret: from-cli") {
			t.Fatalf("tar func.yaml not injected for never-saved function; got:\n%s", streamed)
		}
		if _, err := os.Stat(filepath.Join(root, fn.FunctionFile)); !os.IsNotExist(err) {
			t.Fatalf("sourcesAsTarStream created func.yaml on disk; err=%v", err)
		}
	})
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
