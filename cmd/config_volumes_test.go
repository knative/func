package cmd

import (
	"strings"
	"testing"

	"github.com/ory/viper"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestConfigVolumesAdd_CaseInsensitiveTypes ensures volume types can be provided in any casing
func TestConfigVolumesAdd_CaseInsensitiveTypes(t *testing.T) {
	tests := []struct {
		name       string
		volumeType string
		source     string
		mountPath  string
	}{
		{"ConfigMap uppercase", "ConfigMap", "my-cm", "/etc/cm"},
		{"Secret uppercase", "Secret", "my-sec", "/etc/sec"},
		{"PVC uppercase", "PVC", "my-pvc", "/etc/pvc"},
		{"PersistentVolumeClaim alias", "PersistentVolumeClaim", "my-pvc-alias", "/etc/pvc-alias"},
		{"EmptyDir mixed case", "EmptyDir", "", "/etc/empty"},
		{"emptydir lower case", "emptydir", "", "/etc/emptydir"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			root := FromTempDirectory(t)
			if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			cmd := NewConfigVolumesAddCmd()
			args := []string{"--path=" + root, "--type=" + tc.volumeType, "--mount-path=" + tc.mountPath}
			if tc.source != "" {
				args = append(args, "--source="+tc.source)
			}
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error adding volume with type %q: %v", tc.volumeType, err)
			}

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Run.Volumes) != 1 {
				t.Fatalf("expected 1 volume, got %d", len(f.Run.Volumes))
			}
			if f.Run.Volumes[0].Path == nil || *f.Run.Volumes[0].Path != tc.mountPath {
				t.Fatalf("expected mount path %q, got %v", tc.mountPath, f.Run.Volumes[0].Path)
			}
		})
	}
}

// TestConfigVolumesAdd_DuplicateMountPath ensures adding a duplicate mount path is rejected
func TestConfigVolumesAdd_DuplicateMountPath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigVolumesAddCmd()
	cmd.SetArgs([]string{"--path=" + root, "--type=configmap", "--source=cm1", "--mount-path=/etc/config"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error on first add: %v", err)
	}

	// Try adding second volume with exact same mount path
	viper.Reset()
	cmd2 := NewConfigVolumesAddCmd()
	cmd2.SetArgs([]string{"--path=" + root, "--type=secret", "--source=sec1", "--mount-path=/etc/config"})
	err := cmd2.Execute()
	if err == nil {
		t.Fatal("expected error when adding duplicate mount path, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected error to contain 'already exists', got: %v", err)
	}

	// Try adding with trailing slash variation
	viper.Reset()
	cmd3 := NewConfigVolumesAddCmd()
	cmd3.SetArgs([]string{"--path=" + root, "--type=secret", "--source=sec2", "--mount-path=/etc/config/"})
	err = cmd3.Execute()
	if err == nil {
		t.Fatal("expected error when adding duplicate mount path with trailing slash, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected error to contain 'already exists', got: %v", err)
	}
}

// TestConfigVolumesRemove_CleanMountPath ensures volumes can be removed even if trailing slashes differ
func TestConfigVolumesRemove_CleanMountPath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigVolumesAddCmd()
	cmd.SetArgs([]string{"--path=" + root, "--type=configmap", "--source=cm1", "--mount-path=/etc/config"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error adding volume: %v", err)
	}

	// Remove using trailing slash
	viper.Reset()
	removeCmd := NewConfigVolumesRemoveCmd()
	removeCmd.SetArgs([]string{"--path=" + root, "--mount-path=/etc/config/"})
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("unexpected error removing volume with trailing slash: %v", err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Run.Volumes) != 0 {
		t.Fatalf("expected 0 volumes after removal, got %d", len(f.Run.Volumes))
	}
}

// TestConfigVolumesAdd_EmptyDirMemoryMedium ensures EmptyDir supports case-insensitive Memory medium
func TestConfigVolumesAdd_EmptyDirMemoryMedium(t *testing.T) {
	for _, med := range []string{"Memory", "memory", "MEMORY"} {
		t.Run(med, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			root := FromTempDirectory(t)
			if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			cmd := NewConfigVolumesAddCmd()
			cmd.SetArgs([]string{"--path=" + root, "--type=emptydir", "--mount-path=/tmp/mem", "--medium=" + med})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error with medium %q: %v", med, err)
			}

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Run.Volumes) != 1 || f.Run.Volumes[0].EmptyDir == nil {
				t.Fatal("expected 1 EmptyDir volume")
			}
			if f.Run.Volumes[0].EmptyDir.Medium != fn.StorageMediumMemory {
				t.Fatalf("expected medium %q, got %q", fn.StorageMediumMemory, f.Run.Volumes[0].EmptyDir.Medium)
			}
		})
	}
}

// TestConfigVolumesAdd_InvalidType ensures invalid volume types return a clear error
func TestConfigVolumesAdd_InvalidType(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigVolumesAddCmd()
	cmd.SetArgs([]string{"--path=" + root, "--type=nfs", "--source=share", "--mount-path=/data"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid volume type 'nfs', got nil")
	}
	if !strings.Contains(err.Error(), "invalid volume type") {
		t.Fatalf("expected error to contain 'invalid volume type', got: %v", err)
	}
}
