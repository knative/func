package tar

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func Extract(input io.Reader, destDir string) error {
	var err error

	des, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("cannot read dest dir: %w", err)
	}
	for _, de := range des {
		err = os.RemoveAll(filepath.Join(destDir, de.Name()))
		if err != nil {
			return fmt.Errorf("cannot purge dest dir: %w", err)
		}
	}

	r := tar.NewReader(input)

	var first bool = true
	for {
		var hdr *tar.Header
		hdr, err = r.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if first {
					// mimic tar output on empty input
					return fmt.Errorf("does not look like a tar")
				}
				return nil
			}
			return err
		}
		first = false

		name := hdr.Name
		linkname := hdr.Linkname
		if strings.Contains(name, "..") {
			return fmt.Errorf("name contains '..': %s", name)
		}
		if path.IsAbs(linkname) {
			return fmt.Errorf("absolute symlink: %s->%s", name, linkname)
		}
		if strings.HasPrefix(path.Clean(path.Join(path.Dir(name), linkname)), "..") {
			return fmt.Errorf("link target escapes: %s->%s", name, linkname)
		}

		var destPath, rel string
		destPath = filepath.Join(destDir, filepath.FromSlash(name))
		rel, err = filepath.Rel(destDir, destPath)
		if err != nil {
			return fmt.Errorf("cannot get relative path: %w", err)
		}
		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("name escapes")
		}

		// ensure parent
		err = os.MkdirAll(filepath.Dir(destPath), os.FileMode(hdr.Mode)&fs.ModePerm|0111)
		if err != nil {
			return fmt.Errorf("cannot ensure parent: %w", err)
		}

		switch {
		case hdr.Typeflag == tar.TypeReg:
			err = writeRegularFile(destPath, os.FileMode(hdr.Mode&0777), r)
		case hdr.Typeflag == tar.TypeDir:
			err = os.MkdirAll(destPath, os.FileMode(hdr.Mode)&fs.ModePerm)
		case hdr.Typeflag == tar.TypeSymlink:
			err = os.Symlink(linkname, destPath)
		default:
			_, _ = fmt.Printf("unsupported type flag: %d\n", hdr.Typeflag)
		}
		if err != nil {
			return fmt.Errorf("cannot create entry: %w", err)
		}
	}
}

func writeRegularFile(target string, perm os.FileMode, content io.Reader) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	_, err = io.Copy(f, content)
	if err != nil {
		return err
	}
	return nil
}
