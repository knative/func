package docker

import (
	"bytes"
	"io"
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

// TestToMount_Secret_NamedRun verifies a secret whose name is "run" still
// bind-mounts that directory. The name is not special.
func TestToMount_Secret_NamedRun(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{Secret: ptr("run"), Path: ptr("/etc/secret")}
	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContainerReadable(t, m.Source)
}

// TestToMount_Secret_ExistingModeKept verifies a directory the user already
// created keeps its mode.
func TestToMount_Secret_ExistingModeKept(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, fn.RunDataDir, "run", "secrets", "my-secret")
	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	vol := fn.Volume{Secret: ptr("my-secret"), Path: ptr("/etc/secret")}
	if _, err := toMount(root, vol); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	// Compare with the mode the OS actually stored. Windows does not
	// honor 0700, so an absolute check against that value is meaningless.
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("existing mode changed from %o to %o", before.Mode().Perm(), after.Mode().Perm())
	}
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

// TestToMount_EmptyDir_SizeLimit verifies sizeLimit becomes tmpfs SizeBytes.
func TestToMount_EmptyDir_SizeLimit(t *testing.T) {
	root := t.TempDir()
	limit := "1Mi"
	vol := fn.Volume{EmptyDir: &fn.EmptyDir{SizeLimit: &limit}, Path: ptr("/tmp/scratch")}
	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TmpfsOptions == nil || m.TmpfsOptions.SizeBytes != 1024*1024 {
		t.Fatalf("sizeBytes: got %#v, want %d", m.TmpfsOptions, 1024*1024)
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
	for _, name := range []string{"", ".", "..", "Upper", "foo..bar", "../evil", "../../etc/passwd", "a/b", `a\b`, "../x"} {
		vol := fn.Volume{Secret: ptr(name), Path: ptr("/mnt")}
		if _, err := toMount(root, vol); err == nil {
			t.Errorf("expected error for secret name %q, got nil", name)
		}
	}
}

// TestToMount_Secret_DNSSubdomain verifies a dotted lowercase name is accepted.
// DNS-1123 allows dots that are not a ".." segment.
func TestToMount_Secret_DNSSubdomain(t *testing.T) {
	root := t.TempDir()
	vol := fn.Volume{Secret: ptr("my.secret"), Path: ptr("/mnt")}
	m, err := toMount(root, vol)
	if err != nil {
		t.Fatalf("my.secret should be accepted, got %v", err)
	}
	if !strings.HasSuffix(m.Source, filepath.Join("secrets", "my.secret")) {
		t.Errorf("source %q does not end in the secret name", m.Source)
	}
}

// TestToMount_ConfigMap_TraversalRejected verifies that a configMap name
// containing path traversal sequences is rejected.
func TestToMount_ConfigMap_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "Upper", "foo..bar", "../evil", "../../etc/passwd", "a/b", `a\b`, "../x"} {
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
	binds, mounts := volumeMounts(root, vols, &out)

	if len(binds) != 1 || len(mounts) != 0 {
		t.Fatalf("expected the valid secret on Binds only, binds=%d mounts=%d", len(binds), len(mounts))
	}
	if !strings.HasSuffix(binds[0], ":/ok:ro,z") {
		t.Errorf("bind %q should end with :/ok:ro,z", binds[0])
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
	binds, mounts := volumeMounts(root, vols, &out)

	if len(binds) != 1 || len(mounts) != 0 {
		t.Errorf("expected the valid secret on Binds only, binds=%d mounts=%d", len(binds), len(mounts))
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
	binds, mounts := volumeMounts(root, vols, nil)
	if len(binds) != 1 || len(mounts) != 0 {
		t.Fatalf("expected the secret on Binds only, binds=%d mounts=%d", len(binds), len(mounts))
	}
}

// TestVolumeMounts_BindsAndMounts verifies secret and configMap are Binds
// with ro,z, emptyDir and PVC stay on Mounts, and no target appears twice.
func TestVolumeMounts_BindsAndMounts(t *testing.T) {
	root := t.TempDir()
	limit := "1Mi"
	vols := []fn.Volume{
		{Secret: ptr("my-secret"), Path: ptr("/etc/secret")},
		{ConfigMap: ptr("app-config"), Path: ptr("/etc/config")},
		{EmptyDir: &fn.EmptyDir{SizeLimit: &limit}, Path: ptr("/tmp/scratch")},
		{PersistentVolumeClaim: &fn.PersistentVolumeClaim{ClaimName: ptr("data"), ReadOnly: true}, Path: ptr("/data")},
	}
	binds, mounts := volumeMounts(root, vols, io.Discard)
	if len(binds) != 2 {
		t.Fatalf("binds: got %d (%v), want 2", len(binds), binds)
	}
	if len(mounts) != 2 {
		t.Fatalf("mounts: got %d, want 2", len(mounts))
	}
	secretSrc := filepath.Join(root, fn.RunDataDir, "run", "secrets", "my-secret")
	configSrc := filepath.Join(root, fn.RunDataDir, "run", "configmaps", "app-config")
	wantBinds := map[string]bool{
		secretSrc + ":/etc/secret:ro,z": false,
		configSrc + ":/etc/config:ro,z": false,
	}
	for _, b := range binds {
		if _, ok := wantBinds[b]; !ok {
			t.Errorf("unexpected bind %q", b)
			continue
		}
		wantBinds[b] = true
	}
	for b, seen := range wantBinds {
		if !seen {
			t.Errorf("missing bind %q", b)
		}
	}
	seen := map[string]bool{}
	for _, m := range mounts {
		if m.Type == mount.TypeBind {
			t.Errorf("bind %s must not also be a Mount", m.Target)
		}
		if seen[m.Target] {
			t.Errorf("duplicate mount target %s", m.Target)
		}
		seen[m.Target] = true
	}
	if seen["/etc/secret"] || seen["/etc/config"] {
		t.Errorf("secret or configMap target also present on Mounts: %v", seen)
	}
	if !seen["/tmp/scratch"] || !seen["/data"] {
		t.Errorf("emptyDir or PVC missing from Mounts: %v", seen)
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
	secretDir := filepath.Join(root, fn.RunDataDir, "run", "secrets", "my-secret")
	configDir := filepath.Join(root, fn.RunDataDir, "run", "configmaps", "app-config")
	if !strings.Contains(got, "created empty local secret dir "+secretDir) {
		t.Errorf("expected the secret create line, got %q", got)
	}
	if !strings.Contains(got, "created empty local configmap dir "+configDir) {
		t.Errorf("expected the configmap create line, got %q", got)
	}

	out.Reset()
	volumeMounts(root, vols[:1], &out)
	if strings.Contains(out.String(), "created empty") {
		t.Errorf("an existing directory should not be announced again, got %q", out.String())
	}
}

// assertContainerReadable fails when a non-root container user could not
// traverse the bind. 0755 has the other read and execute bits; 0700 does not.
func assertContainerReadable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0055 != 0055 {
		t.Errorf("%s mode %o is not readable by a non-root container user", path, info.Mode().Perm())
	}
}
