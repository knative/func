package rsync_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"knative.dev/func/rsync"
)

func TestSyncBasic(t *testing.T) {
	var err error
	dirA := t.TempDir()

	r := rand.New(rand.NewSource(0))

	sendErr, recvErr := runSync(t, "testdata", dirA)
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	if recvErr != nil {
		t.Fatal(recvErr)
	}

	a, err := loadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	b, err := loadDir(dirA)
	if err != nil {
		t.Fatal(err)
	}

	d := cmp.Diff(a, b)
	if d != "" {
		t.Error("content of directories does not match (-want, +got):\n", d)
	}

	data := make([]byte, 16*(1<<10))
	_, err = r.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dirA, "a.bin"), data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	data[15*(1<<10)]++

	dirB := t.TempDir()
	err = os.WriteFile(filepath.Join(dirB, "a.bin"), data, 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chtimes(filepath.Join(dirB, "a.bin"), time.Now(), time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	sendErr, recvErr = runSync(t, dirA, dirB)
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	if recvErr != nil {
		t.Fatal(recvErr)
	}

	a, err = loadDir(dirA)
	if err != nil {
		t.Fatal(err)
	}

	b, err = loadDir(dirB)
	if err != nil {
		t.Fatal(err)
	}

	d = cmp.Diff(a, b)
	if d != "" {
		t.Error("content of directories does not match (-want, +got):\n", d)
	}

	err = os.Remove(filepath.Join(dirA, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}

	sendErr, recvErr = runSync(t, dirA, dirB)
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	if recvErr != nil {
		t.Fatal(recvErr)
	}

	a, err = loadDir(dirA)
	if err != nil {
		t.Fatal(err)
	}

	b, err = loadDir(dirB)
	if err != nil {
		t.Fatal(err)
	}

	d = cmp.Diff(a, b)
	if d != "" {
		t.Error("content of directories does not match (-want, +got):\n", d)
	}
}

func TestSyncMany(t *testing.T) {
	var err error
	var a, b []file
	dirA := t.TempDir()
	dirB := t.TempDir()

	generateFiles(t, dirA, dirB)

	sendErr, recvErr := runSync(t, dirA, dirB)
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	if recvErr != nil {
		t.Fatal(recvErr)
	}

	a, err = loadDir(dirA)
	if err != nil {
		t.Fatal(err)
	}

	b, err = loadDir(dirB)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Error("content of directories does not match")
	}
}

func TestSyncReceiverError(t *testing.T) {
	var err error
	dirA := t.TempDir()
	dirB := t.TempDir()

	generateFiles(t, dirA, dirB)

	des, err := os.ReadDir(dirB)
	if err != nil {
		t.Fatal(err)
	}

	// receiver won't be able to compute signature of this file
	err = os.Chmod(filepath.Join(dirB, des[len(des)/2].Name()), 0000)
	if err != nil {
		t.Fatal(err)
	}

	sendErr, recvErr := runSync(t, dirA, dirB)
	t.Log("send error:", sendErr)
	t.Log("recv error:", recvErr)

	t.Skip("TODO: fix this")

	if (sendErr == nil) != (recvErr == nil) {
		t.Error("sender and receiver error state mismatch")
	}
	if !errors.Is(sendErr, rsync.RemoteError) {
		t.Errorf("expected RemoteErr on sender end, but got: %v", sendErr)
	}
	if recvErr == nil {
		t.Error("expected error on receiver end")
	}
}

func TestSyncSenderError(t *testing.T) {
	var err error
	dirA := t.TempDir()
	dirB := t.TempDir()

	generateFiles(t, dirA, dirB)

	des, err := os.ReadDir(dirB)
	if err != nil {
		t.Fatal(err)
	}

	// sender won't be able to compute delta of this file
	err = os.Chmod(filepath.Join(dirA, des[len(des)/2].Name()), 0000)
	if err != nil {
		t.Fatal(err)
	}

	sendErr, recvErr := runSync(t, dirA, dirB)
	t.Log("send error:", sendErr)
	t.Log("recv error:", recvErr)

	t.Skip("TODO: fix this")

	if (sendErr == nil) != (recvErr == nil) {
		t.Error("sender and receiver error state mismatch")
	}
	if !errors.Is(recvErr, rsync.RemoteError) {
		t.Errorf("expected RemoteErr on sender end, but got: %v", sendErr)
	}
	if sendErr == nil {
		t.Error("expected error on receiver end")
	}
}

func TestPermDenied(t *testing.T) {
	dir := "/etc/"
	if runtime.GOOS == "windows" {
		dir = "C:\\Windows\\"
	}
	sendErr, recvErr := runSync(t, "testdata", dir)
	t.Log("send error:", sendErr)
	t.Log("recv error:", recvErr)

	t.Skip("TODO: fix this")

	if (sendErr == nil) != (recvErr == nil) {
		t.Error("sender and receiver error state mismatch")
	}
	if !errors.Is(sendErr, rsync.RemoteError) {
		t.Errorf("expected RemoteErr on sender end, but got: %v", sendErr)
	}
	if recvErr == nil {
		t.Error("expected error on receiver end")
	}
}

func generateFiles(t *testing.T, dirA, dirB string) {
	t.Helper()
	var err error
	r := rand.New(rand.NewSource(0))

	buff := make([]byte, 1<<20)

	var size int
	for i := 0; i < 1024; i++ {
		data := buff[:r.Intn((32*(1<<10))-512)+512]
		size += len(data)
		r.Read(data)
		name := fmt.Sprintf("a_%04d.bin", i)
		err = os.WriteFile(filepath.Join(dirB, name), data, 0644)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Chtimes(filepath.Join(dirB, name), time.Now(), time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}

		chCnt := r.Intn(5)
		for j := 0; j < chCnt; j++ {
			data[r.Intn(len(data))]++
		}

		err = os.WriteFile(filepath.Join(dirA, name), data, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 64; i++ {
		data := buff[:r.Intn((32*(1<<10))-512)+512]
		size += len(data)
		err = os.WriteFile(filepath.Join(dirA, fmt.Sprintf("a_new_%04d.bin", i)), data, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("generated size: %dB", size)
}

func runSync(t *testing.T, src, dest string) (error, error) {
	t.Helper()
	start := time.Now()
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()

	var eg errgroup.Group
	eg.Go(func() error {
		conn := &conn{PipeReader: pr1, PipeWriter: pw2}
		err := rsync.ReceiveFiles(conn, dest)
		if err != nil {
			return fmt.Errorf("receive failed: %w", err)
		}
		return nil
	})
	forEachFile := func(processFile rsync.ProcessFile) error {
		return filepath.Walk(src, func(path string, fi fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relp, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			if relp == "." {
				return nil
			}
			return processFile(path, filepath.ToSlash(relp), fi, err)
		})
	}
	conn := &conn{PipeReader: pr2, PipeWriter: pw1}
	serr := rsync.SendFiles(conn, forEachFile)
	rerr := eg.Wait()
	t.Logf("Stats: Tx: %dB Rx: %dB", conn.written, conn.read)
	t.Log("Sync took: ", time.Since(start))
	return serr, rerr
}

type file struct {
	Name    string
	Mode    fs.FileMode
	Content []byte
}

func loadDir(root string) ([]file, error) {
	var err error
	var files []file
	err = filepath.Walk(root, func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rp, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		f := file{
			Name: rp,
			Mode: fi.Mode(),
		}
		switch {
		case fi.Mode().IsDir():
			f.Content = nil
		case fi.Mode().Type() == 0:
			f.Content, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		case fi.Mode().Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(path)
			if err != nil {
				return err
			}
			f.Content = []byte(t)
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

type conn struct {
	*io.PipeReader
	*io.PipeWriter
	written, read int64
}

func (c *conn) Read(b []byte) (int, error) {
	n, err := c.PipeReader.Read(b)
	atomic.AddInt64(&c.read, int64(n))
	return n, err
}

func (c *conn) Write(b []byte) (int, error) {
	n, err := c.PipeWriter.Write(b)
	atomic.AddInt64(&c.written, int64(n))
	return n, err
}
func (c *conn) Close() error {
	c.PipeReader.Close()
	c.PipeWriter.Close()
	return nil
}
