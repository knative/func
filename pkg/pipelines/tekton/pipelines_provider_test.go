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

func Test_createPipelinePersistentVolumeClaim(t *testing.T) {
	type mockCreateType func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error)
	type mockGetType func(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error)
	type mockDeleteType func(ctx context.Context, name, namespaceOverride string) error
	type mockWaitType func(ctx context.Context, name, namespaceOverride string) error

	type args struct {
		ctx       context.Context
		f         fn.Function
		namespace string
		labels    map[string]string
		size      string
	}
	tests := []struct {
		name       string
		args       args
		mockCreate mockCreateType
		mockGet    mockGetType
		mockDelete mockDeleteType
		mockWait   mockWaitType
		wantErr    bool
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
			mockGet: func(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
				return nil, &apiErrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
			},
			mockCreate: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return errors.New("creation of pvc failed")
			},
			wantErr: true,
		},
		{
			name: "deletes and recreates if pvc already exists",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mockGet: func(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
				return &corev1.PersistentVolumeClaim{}, nil
			},
			mockDelete: func(ctx context.Context, name, namespaceOverride string) error {
				return nil
			},
			mockWait: func(ctx context.Context, name, namespaceOverride string) error {
				return nil
			},
			mockCreate: func(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity, storageClass string) (err error) {
				return nil
			},
			wantErr: false,
		},
		{
			name: "returns error if deletion fails",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mockGet: func(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
				return &corev1.PersistentVolumeClaim{}, nil
			},
			mockDelete: func(ctx context.Context, name, namespaceOverride string) error {
				return errors.New("deletion failed")
			},
			wantErr: true,
		},
		{
			name: "returns error if waiting for deletion fails",
			args: args{
				ctx:       t.Context(),
				f:         fn.Function{},
				namespace: "test-ns",
				labels:    nil,
				size:      DefaultPersistentVolumeClaimSize.String(),
			},
			mockGet: func(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
				return &corev1.PersistentVolumeClaim{}, nil
			},
			mockDelete: func(ctx context.Context, name, namespaceOverride string) error {
				return nil
			},
			mockWait: func(ctx context.Context, name, namespaceOverride string) error {
				return errors.New("wait for deletion failed")
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// save current functions and restore them at the end
			oldCreate := createPersistentVolumeClaim
			oldGet := getPersistentVolumeClaim
			oldDelete := deletePersistentVolumeClaim
			oldWait := waitForPVCDeletion
			defer func() {
				createPersistentVolumeClaim = oldCreate
				getPersistentVolumeClaim = oldGet
				deletePersistentVolumeClaim = oldDelete
				waitForPVCDeletion = oldWait
			}()

			if tt.mockCreate != nil {
				createPersistentVolumeClaim = tt.mockCreate
			}
			if tt.mockGet != nil {
				getPersistentVolumeClaim = tt.mockGet
			}
			if tt.mockDelete != nil {
				deletePersistentVolumeClaim = tt.mockDelete
			}
			if tt.mockWait != nil {
				waitForPVCDeletion = tt.mockWait
			}

			tt.args.f.Build.PVCSize = tt.args.size
			if err := createPipelinePersistentVolumeClaim(tt.args.ctx, tt.args.f, tt.args.namespace, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("createPipelinePersistentVolumeClaim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
