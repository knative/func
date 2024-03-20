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
	root := filepath.Join("testdata", "fn-src")

	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
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

func Test_createPipelinePersistentVolumeClaim(t *testing.T) {
	type mockType func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error)

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
				ctx:       context.Background(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error) {
				return errors.New("creation of pvc failed")
			},
			wantErr: true,
		},
		{
			name: "returns nil if pvc already exists",
			args: args{
				ctx:       context.Background(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error) {
				return &apiErrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonAlreadyExists}}
			},
			wantErr: false,
		},
		{
			name: "returns err if namespace not defined and default returns an err",
			args: args{
				ctx:       context.Background(),
				f:         fn.Function{},
				namespace: "",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mock: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error) {
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
