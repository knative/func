package cluster

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromTarGz_happyPath(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "tool.tar.gz")
	writeTarGz(t, archive, map[string][]byte{
		"LICENSE": []byte("mit"),
		"act":     []byte("#!/bin/sh\necho act\n"),
	})

	if err := extractFromTarGz(archive, "act"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho act\n" {
		t.Fatalf("extracted body = %q", got)
	}
}

func TestExtractFromTarGz_missingEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "tool.tar.gz")
	writeTarGz(t, archive, map[string][]byte{"other": []byte("x")})

	if err := extractFromTarGz(archive, "act"); err == nil {
		t.Fatal("expected missing-entry error")
	}
}

func writeTarGz(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
