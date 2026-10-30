package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/mount"

	fn "knative.dev/func/pkg/functions"
)

func ptr(s string) *string { return &s }

// TestToMount_Secret verifies that a Secret volume produces a bind mount
// rooted inside <root>/.func/run/secrets/<name>.
func TestToMount_Secret(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{Secret: ptr("my-secret"), Path: ptr("/etc/secret")}

	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != mount.TypeBind {
		t.Errorf("expected TypeBind, got %v", m.Type)
	}
	want := filepath.Join(root, fn.RunDataDir, "run", "secrets", "my-secret")
	if m.Source != want {
		t.Errorf("source: got %q, want %q", m.Source, want)
	}
	if m.Target != "/etc/secret" {
		t.Errorf("target: got %q, want %q", m.Target, "/etc/secret")
	}
	if !m.ReadOnly {
		t.Error("secret mount should be read-only, matching Kubernetes")
	}
	assertContainerReadable(t, m.Source)
}

// TestToMount_ConfigMap verifies that a ConfigMap volume produces a bind mount
// rooted inside <root>/.func/run/configmaps/<name>.
func TestToMount_ConfigMap(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{ConfigMap: ptr("app-config"), Path: ptr("/etc/config")}

	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != mount.TypeBind {
		t.Errorf("expected TypeBind, got %v", m.Type)
	}
	want := filepath.Join(root, fn.RunDataDir, "run", "configmaps", "app-config")
	if m.Source != want {
		t.Errorf("source: got %q, want %q", m.Source, want)
	}
	if m.Target != "/etc/config" {
		t.Errorf("target: got %q, want %q", m.Target, "/etc/config")
	}
	if !m.ReadOnly {
		t.Error("configMap mount should be read-only, matching Kubernetes")
	}
	assertContainerReadable(t, m.Source)
}

// TestToMount_EmptyDir_Memory verifies that an EmptyDir with Memory medium maps to tmpfs.
func TestToMount_EmptyDir_Memory(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{EmptyDir: &fn.EmptyDir{Medium: fn.StorageMediumMemory}, Path: ptr("/tmp/cache")}

	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != mount.TypeTmpfs {
		t.Errorf("expected TypeTmpfs for Memory medium, got %v", m.Type)
	}
	if m.Target != "/tmp/cache" {
		t.Errorf("target: got %q, want %q", m.Target, "/tmp/cache")
	}
}

// TestToMount_EmptyDir_Default verifies that an EmptyDir with default medium also maps
// to tmpfs (not an anonymous volume), to match ephemeral pod semantics and avoid leaks.
func TestToMount_EmptyDir_Default(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{EmptyDir: &fn.EmptyDir{}, Path: ptr("/tmp/scratch")}

	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != mount.TypeTmpfs {
		t.Errorf("expected TypeTmpfs for default medium, got %v", m.Type)
	}
}

// TestToMount_PVC verifies that a PVC volume produces a named Docker volume
// keyed by claimName, so multiple functions referencing the same claim share data.
func TestToMount_PVC(t *testing.T) {
	root := t.TempDir()
	claim := "my-claim"
	vol := fn.Volume{
		PersistentVolumeClaim: &fn.PersistentVolumeClaim{ClaimName: ptr(claim)},
		Path:                  ptr("/data"),
	}

	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != mount.TypeVolume {
		t.Errorf("expected TypeVolume for PVC, got %v", m.Type)
	}
	if m.Source != claim {
		t.Errorf("source: got %q, want %q", m.Source, claim)
	}
	if m.Target != "/data" {
		t.Errorf("target: got %q, want %q", m.Target, "/data")
	}
}

// TestToMount_PVC_ReadOnly verifies that ReadOnly=true is forwarded to the mount.
func TestToMount_PVC_ReadOnly(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{
		PersistentVolumeClaim: &fn.PersistentVolumeClaim{ClaimName: ptr("ro-claim"), ReadOnly: true},
		Path:                  ptr("/ro"),
	}
	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.ReadOnly {
		t.Error("expected mount to be ReadOnly")
	}
}

// TestToMount_PVC_MissingClaimName verifies an error is returned when claimName is nil.
func TestToMount_PVC_MissingClaimName(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{
		PersistentVolumeClaim: &fn.PersistentVolumeClaim{},
		Path:                  ptr("/data"),
	}
	if _, err := toMount(root, vol); err == nil {
		t.Error("expected error for missing claimName, got nil")
	}
}

// TestToMount_UnrecognizedType verifies an error is returned for a volume with no type set.
func TestToMount_UnrecognizedType(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{Path: ptr("/some/path")}
	if _, err := toMount(root, vol); err == nil {
		t.Error("expected error for unrecognized volume type, got nil")
	}
}

// TestToMount_Secret_TraversalRejected verifies that a secret name containing
// path traversal sequences is rejected to prevent escaping the .func tree.
func TestToMount_Secret_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", "..", "../evil", "../../etc/passwd", "a/b", `a\b`} {
		vol := fn.Volume{Secret: ptr(name), Path: ptr("/mnt")}
		if _, err := toMount(root, vol); err == nil {
			t.Errorf("expected error for secret name %q, got nil", name)
		}
	}
}

// TestToMount_Secret_ConsecutiveDotsAllowed verifies that ".." as a substring
// is not treated as traversal. foo..bar is a legal name.
func TestToMount_Secret_ConsecutiveDotsAllowed(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{Secret: ptr("foo..bar"), Path: ptr("/mnt")}
	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("foo..bar should be allowed, got %v", err)
	}
	if !strings.HasSuffix(m.Source, filepath.Join("secrets", "foo..bar")) {
		t.Errorf("source %q does not end in the secret name", m.Source)
	}
}

// TestToMount_ConfigMap_TraversalRejected verifies that a configMap name
// containing path traversal sequences is rejected.
func TestToMount_ConfigMap_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", "..", "../evil", "../../etc/passwd", "a/b", `a\b`} {
		vol := fn.Volume{ConfigMap: ptr(name), Path: ptr("/mnt")}
		if _, err := toMount(root, vol); err == nil {
			t.Errorf("expected error for configMap name %q, got nil", name)
		}
	}
}

// TestVolumeMounts_SkipsNilPath verifies that volumes missing a path emit a warning
// and are skipped without aborting.
func TestVolumeMounts_SkipsNilPath(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	vols := []fn.Volume{
		{Secret: ptr("s"), Path: nil},         // no path — should be skipped
		{Secret: ptr("s2"), Path: ptr("/ok")}, // valid
	}
	mounts := volumeMounts(root, vols, &out)

	if len(mounts) != 1 {
		t.Errorf("expected 1 mount, got %d", len(mounts))
	}
	if !strings.Contains(out.String(), "missing path") {
		t.Errorf("expected warning about missing path, got: %q", out.String())
	}
}

// TestVolumeMounts_SkipsOnError verifies that a volume that produces an error is
// warned about and skipped, while valid volumes are still mounted.
func TestVolumeMounts_SkipsOnError(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	vols := []fn.Volume{
		// PVC with no claimName — toMount returns error
		{PersistentVolumeClaim: &fn.PersistentVolumeClaim{}, Path: ptr("/bad")},
		// Valid secret
		{Secret: ptr("good"), Path: ptr("/good")},
	}
	mounts := volumeMounts(root, vols, &out)

	if len(mounts) != 1 {
		t.Errorf("expected 1 mount, got %d", len(mounts))
	}
	if !strings.Contains(out.String(), "warning") {
		t.Errorf("expected warning in output, got: %q", out.String())
	}
}

// TestToMount_NilPath verifies the helper rejects a nil path itself, not only
// via volumeMounts.
func TestToMount_NilPath(t *testing.T) {
	if _, err := toMount(t.TempDir(), fn.Volume{Secret: ptr("s")}); err == nil {
		t.Fatal("expected error for nil path")
	}
}

// TestVolumeMounts_NilWriter verifies a nil warning writer does not panic.
func TestVolumeMounts_NilWriter(t *testing.T) {
	root := t.TempDir()
	vols := []fn.Volume{{Secret: ptr("s"), Path: ptr("/ok")}}
	mounts := volumeMounts(root, vols, nil)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
}

// TestVolumeMounts_HintsWhenEmpty verifies an empty secret or configMap
// directory tells the user to put files there.
func TestVolumeMounts_HintsWhenEmpty(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	vols := []fn.Volume{
		{Secret: ptr("my-secret"), Path: ptr("/etc/secret")},
		{ConfigMap: ptr("app-config"), Path: ptr("/etc/config")},
	}
	volumeMounts(root, vols, &out)
	got := out.String()
	if !strings.Contains(got, "hint:") || !strings.Contains(got, "my-secret") {
		t.Errorf("expected a secret hint, got %q", got)
	}
	if !strings.Contains(got, "app-config") {
		t.Errorf("expected a configMap hint, got %q", got)
	}

	secretDir := filepath.Join(root, fn.RunDataDir, "run", "secrets", "my-secret")
	if err := os.WriteFile(filepath.Join(secretDir, "token"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	volumeMounts(root, vols[:1], &out)
	if strings.Contains(out.String(), "hint:") {
		t.Errorf("populated secret should not hint, got %q", out.String())
	}
}

// assertContainerReadable fails when a non-root container user could not
// traverse the bind. 0755 has the other read and execute bits; 0700 does not.
func assertContainerReadable(t *testing.T, path string) {
	t.Helper()
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm()&0055 != 0055 {
			t.Errorf("%s mode %o is not readable by a non-root container user", p, info.Mode().Perm())
		}
		if filepath.Base(p) == "run" {
			return
		}
		if filepath.Dir(p) == p {
			t.Fatalf("%s is not under a run directory", path)
		}
	}
}
