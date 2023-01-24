package v1

import (
	"bytes"
	"io/fs"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestWire(t *testing.T) {
	var buff bytes.Buffer
	var err error
	var d string

	fi := FileInfo{
		Path:    "dir/a.lnk♥",
		Size:    1024,
		Mode:    0755 | fs.ModeSymlink,
		ModTime: time.Unix(1, 500_000_000),
		Link:    "dir/a.txt♥",
	}
	buff.Reset()
	err = writeFileInfo(&buff, fi)
	if err != nil {
		t.Fatal(err)
	}

	fi2, err := readFileInfo(&buff)
	if err != nil {
		t.Fatal(err)
	}
	d = cmp.Diff(&fi, &fi2)
	if d != "" {
		t.Error("file info serde mismatch:\n", d)
	}

}
