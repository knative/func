package tar_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	tarutil "knative.dev/func/pkg/tar"
)

const (
	aTxt1 = "a.txt first revision"
	bTxt1 = "b.txt first revision"
	aTxt2 = "a.txt second revision"
	bTxt2 = "b.txt second revision"
)

func TestExtract(t *testing.T) {
	var err error
	d := t.TempDir()
	err = tarutil.Extract(tarballV1(t), d)
	if err != nil {
		t.Fatal(err)
	}

	bs, err := os.ReadFile(filepath.Join(d, "dir/a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(bs)
	if s != aTxt1 {
		t.Errorf("unexpected data: %s", s)
	}
	bs, err = os.ReadFile(filepath.Join(d, "dir/b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s = string(bs)
	if s != bTxt1 {
		t.Errorf("unexpected data: %s", s)
	}

	err = tarutil.Extract(tarballV2(t), d)
	if err != nil {
		t.Fatal(err)
	}

	bs, err = os.ReadFile(filepath.Join(d, "dir/a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s = string(bs)
	if s != aTxt2 {
		t.Errorf("unexpected data: %s", s)
	}
	bs, err = os.ReadFile(filepath.Join(d, "dir/b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s = string(bs)
	if s != bTxt2 {
		t.Errorf("unexpected data: %s", s)
	}
}

func tarballV1(t *testing.T) io.Reader {
	t.Helper()

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/a.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(aTxt1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(aTxt1))
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/data1",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(bTxt1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(bTxt1))
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/b.txt",
		Linkname: "data1",
		Typeflag: tar.TypeSymlink,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &buff
}

func tarballV2(t *testing.T) io.Reader {
	t.Helper()

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/a.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(aTxt2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(aTxt2))
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/b.txt",
		Linkname: "data2",
		Typeflag: tar.TypeSymlink,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "dir/data2",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(bTxt2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(bTxt2))
	if err != nil {
		t.Fatal(err)
	}

	return &buff
}
