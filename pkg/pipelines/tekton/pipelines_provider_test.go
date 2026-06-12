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
	"knative.dev/func/pkg/k8s"
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
	type mockType func(ctx context.Context, kc *k8s.Client, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error)

	type args struct {
		ctx       context.Context
		kc        *k8s.Client
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
			mock: func(ctx context.Context, kc *k8s.Client, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
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
			mock: func(ctx context.Context, kc *k8s.Client, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
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
			mock: func(ctx context.Context, kc *k8s.Client, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return errors.New("no namespace defined")
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := createPersistentVolumeClaim
			defer func() { createPersistentVolumeClaim = old }()

			createPersistentVolumeClaim = tt.mock
			tt.args.f.Build.PVCSize = tt.args.size
			if err := createPipelinePersistentVolumeClaim(tt.args.ctx, tt.args.kc, tt.args.f, tt.args.namespace, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("createPipelinePersistentVolumeClaim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
