package tar_test

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	tarutil "knative.dev/func/pkg/tar"
)

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name         string
		createReader func(*testing.T) io.Reader
		wantErr      bool
	}{
		{
			name:         "non escaping link",
			createReader: nonEscapingSymlink,
			wantErr:      false,
		},
		{
			name:         "missing parent of regular file",
			createReader: missingParentRegular,
			wantErr:      false,
		},
		{
			name:         "missing parent of link",
			createReader: missingParentLink,
			wantErr:      false,
		},

		{
			name:         "absolute symlink",
			createReader: absoluteSymlink,
			wantErr:      true,
		},
		{
			name:         "direct escape",
			createReader: directEscape,
			wantErr:      true,
		},
		{
			name:         "indirect link escape",
			createReader: indirectLinkEscape,
			wantErr:      true,
		},
		{
			name:         "indirect link escape with overwrite",
			createReader: indirectLinkEscapeWithOverwrites,
			wantErr:      true,
		},
		{
			name:         "double dot in name",
			createReader: doubleDotInName,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			if err := tarutil.Extract(tt.createReader(t), d); (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func nonEscapingSymlink(t *testing.T) io.Reader {
	t.Helper()

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent",
		Linkname: "..",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &buff

}

func absoluteSymlink(t *testing.T) io.Reader {
	var err error
	var buff bytes.Buffer
	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "a.lnk",
		Linkname: "/etc/shadow",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &buff
}

func directEscape(t *testing.T) io.Reader {
	t.Helper()

	var err error
	var buff bytes.Buffer
	var msg = "I am free!!!"
	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "../escaped.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(msg)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	return &buff
}

func indirectLinkEscape(t *testing.T) io.Reader {
	t.Helper()
	t.Skip("we are not checking for this since the core utils tar does not too")

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent",
		Linkname: "..",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteHeader(&tar.Header{
		Name:     "escape",
		Linkname: "subdir/parent/..",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &buff
}

func indirectLinkEscapeWithOverwrites(t *testing.T) io.Reader {
	t.Helper()
	t.Skip("we are not checking for this since the core utils tar does not too")

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "escape",
		Linkname: "subdir/parent/..",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent",
		Linkname: "..",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &buff
}

func doubleDotInName(t *testing.T) io.Reader {
	t.Helper()

	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent",
		Typeflag: tar.TypeDir,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteHeader(&tar.Header{
		Name:     "subdir/parent/../../a.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &buff
}

func missingParentRegular(t *testing.T) io.Reader {
	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "missing/a.txt",
		Typeflag: tar.TypeReg,
		Size:     0,
		Mode:     0644,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &buff
}

func missingParentLink(t *testing.T) io.Reader {
	var err error
	var buff bytes.Buffer

	w := tar.NewWriter(&buff)
	defer func(w *tar.Writer) {
		_ = w.Close()
	}(w)
	err = w.WriteHeader(&tar.Header{
		Name:     "missing/a.lnk",
		Linkname: "a.txt",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &buff
}
