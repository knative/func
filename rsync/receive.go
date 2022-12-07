package rsync

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/balena-os/librsync-go"
)

func ReceiveFiles(conn io.ReadWriteCloser, root string) error {

	var files = make([]FileInfo, 0, 32*1<<10)
	var paths = make([]string, 0, 32*1<<10)
	r := bufio.NewReader(conn)

	//region Receive file list
	for {
		f, err := readFileInfo(r)
		if err != nil {
			return fmt.Errorf("cannot read file from input: %w", err)
		}
		if f.Path == fileListEndSentinel {
			break
		}
		files = append(files, f)
		paths = append(paths, filepath.Join(root, filepath.FromSlash(f.Path)))
	}
	//endregion Receive file list

	eg, ctx := errgroup.WithContext(context.Background())
	differingFiles := make(chan pathIdPair, chanBuffSize)
	missingFiles := make(chan uint32, chanBuffSize)
	signatures := make(chan dataChunk, chanBuffSize)
	deltas := make(chan readerIdPair, chanBuffSize)

	//region Delete files
	eg.Go(func() error {
		fileSet := make(map[string]bool, len(paths))
		for _, p := range paths {
			fileSet[p] = true
		}
		return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}
			if _, ok := fileSet[path]; !ok {
				err = os.RemoveAll(path)
				if err != nil {
					return err
				}
			}
			return nil
		})
	})
	//endregion Delete files

	//region Process file list
	eg.Go(func() error {
		var err error
		defer func() {
			close(differingFiles)
			close(missingFiles)
		}()
		for id, f := range files {
			err = processFile(ctx, uint32(id), f, missingFiles, differingFiles, root)
			if err != nil {
				return fmt.Errorf("cannot process file: %w", err)
			}
		}
		return nil
	})
	//endregion Process file list

	//region Generate signatures
	signatureWorkerCount := int32(workerCount)
	for i := 0; i < workerCount; i++ {
		eg.Go(func() error {
			var err error
			defer func() {
				cnt := atomic.AddInt32(&signatureWorkerCount, -1)
				if cnt == 0 {
					close(signatures)
				}
			}()
			for p := range differingFiles {
				err = createSignature(ctx, p.id, p.path, signatures)
				if err != nil {
					return fmt.Errorf("cannot create signature: %w", err)
				}
			}
			return nil
		})
	}
	//endregion Generate signatures

	//region Apply deltas
	deltaWorkerCount := int32(workerCount)
	for i := 0; i < workerCount; i++ {
		eg.Go(func() error {
			var err error

			defer func() {
				cnt := atomic.AddInt32(&deltaWorkerCount, -1)
				if cnt == 0 {
					for p := range deltas {
						_ = p.r.CloseWithError(context.Canceled)
					}
				}
			}()

			for p := range deltas {
				err = applyPatch(p.r, files[p.id], paths[p.id])
				if err != nil {
					err = fmt.Errorf("cannot patch: %w", err)
					_ = p.r.CloseWithError(err)
					return err
				}
				_ = p.r.Close()
			}
			return nil
		})
	}
	//endregion Apply deltas

	//region Handle outcoming data
	eg.Go(func() error {
		var err error
		var buff [8]byte
		w := bufio.NewWriter(conn)
		writeTypeAndId := func(msgType messageType, id uint32) error {
			buff[0] = msgType
			binary.BigEndian.PutUint32(buff[1:], id)
			_, err = w.Write(buff[:5])
			if err != nil {
				return err
			}
			return nil
		}
		for {
			select {
			case id, ok := <-missingFiles:
				if !ok {
					missingFiles = nil
					break
				}
				err = writeTypeAndId(fileData, id)
				if err != nil {
					return fmt.Errorf("cannot write file requst: %w", err)
				}
			case p, ok := <-signatures:
				if !ok {
					signatures = nil
					break
				}
				err = writeTypeAndId(signatureData, p.id)
				if err != nil {
					return fmt.Errorf("cannot write signature data: %w", err)
				}
				err = writeByteArray(w, p.data)
				if err != nil {
					return fmt.Errorf("cannot write signature data: %w", err)
				}
			}

			if signatures == nil && missingFiles == nil {
				err = w.WriteByte(endOfExchange)
				if err != nil {
					return fmt.Errorf("cannot write end of exchange: %w", err)
				}
				err = w.Flush()
				if err != nil {
					return fmt.Errorf("cannot flush data: %w", err)
				}
				return nil
			}
		}
	})
	//endregion Handle outcoming data

	//region Handle incoming data
	deltaWriters := make(map[uint32]*io.PipeWriter)
	var buff = make([]byte, 32*(1<<10))
	for {
		var msgType messageType
		var err error
		msgType, err = r.ReadByte()
		if err != nil {
			return fmt.Errorf("cannot read message type from input: %w", err)
		}

		if msgType == endOfExchange {
			break
		}

		var id uint32
		_, err = io.ReadFull(r, buff[:4])
		if err != nil {
			return fmt.Errorf("cannot read file id: %w", err)
		}
		id = binary.BigEndian.Uint32(buff[:4])

		switch msgType {
		case fileData:
			err = receiveFile(r, files[id], paths[id], buff)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "cannot download file: %v", err)
				continue
			}
		case deltaData:
			data, err := readByteArray(r, buff)
			if err != nil {
				return fmt.Errorf("cannot read delta: %w", err)
			}

			select {
			case <-ctx.Done():
				continue
			default:
			}

			deltaWriter, ok := deltaWriters[id]
			if !ok {
				pr, pw := io.Pipe()
				select {
				case deltas <- readerIdPair{id: id, r: pr}:
				case <-ctx.Done():
					continue
				}
				deltaWriter = pw
				deltaWriters[id] = pw
			}

			if len(data) > 0 {
				_, err = deltaWriter.Write(data)
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "cannot write delta chunk: %v\n", err)
					continue
				}
			} else {
				delete(deltaWriters, id)
				err = deltaWriter.Close()
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "cannot close delta writer: %v", err)
					continue
				}
			}
		default:
			panic(fmt.Sprintf("unknown message type: 0x%x\n", msgType))
		}
	}
	close(deltas)
	for _, pw := range deltaWriters {
		_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	}
	//endregion Handle incoming data

	err := eg.Wait()
	if err != nil {
		return err
	}

	return nil
}

func processFile(ctx context.Context, id uint32, fi FileInfo, missingFiles chan<- uint32, differingFiles chan<- pathIdPair, root string) error {
	var err error
	var localFileInfo fs.FileInfo

	localPath := filepath.Join(root, filepath.FromSlash(fi.Path))
	localFileInfo, err = os.Lstat(localPath)
	existsLoc := true
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot lstat local file: %w", err)
		}
		existsLoc = false
		localFileInfo = nil
	}

	if existsLoc &&
		localFileInfo.Mode().Type() == fi.Mode.Type() &&
		localFileInfo.ModTime().Equal(fi.ModTime) &&
		(localFileInfo.Size() == fi.Size && fi.Mode.Type() == 0) {
		// skipping the file is already up-to-date
		return nil
	}

	// remove if file exist locally, but type doesn't match
	if existsLoc && localFileInfo.Mode().Type() != fi.Mode.Type() {
		err = os.Remove(localPath)
		if err != nil {
			return fmt.Errorf("cannot remove file: %w", err)
		}
		existsLoc = false
		localFileInfo = nil
	}

	if existsLoc && localFileInfo.Mode().Type() == 0 && localFileInfo.Size() == 0 {
		err = os.Remove(localPath)
		if err != nil {
			return fmt.Errorf("cannot remove file: %w", err)
		}
		existsLoc = false
		localFileInfo = nil
	}

	switch {
	case fi.Mode.IsDir():
		err = os.MkdirAll(localPath, 0755)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("cannot create dir: %w", err)
		}
		err = os.Chmod(localPath, 0755)
		if err != nil {
			return fmt.Errorf("cannot set mod of dir: %w", err)
		}
	case fi.Mode.Type()&fs.ModeSymlink != 0:
		err = os.Symlink(fi.Link, localPath)
		if err != nil {
			if !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("cannot create symlink: %w", err)
			}
			var oldTarget string
			oldTarget, err = os.Readlink(localPath)
			if err != nil {
				return fmt.Errorf("cannot read symlink old target: %w", err)
			}
			// TODO re-set symlink atomically
			if oldTarget != fi.Link {
				err = os.Remove(localPath)
				if err != nil {
					return fmt.Errorf("cannot remove symlink: %w", err)
				}
				err = os.Symlink(fi.Link, localPath)
				if err != nil {
					return fmt.Errorf("cannot create symlink: %w", err)
				}
			}
		}
	case fi.Mode.Type() == 0:
		if !existsLoc {
			select {
			case <-ctx.Done():
				return context.Canceled
			case missingFiles <- id:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case differingFiles <- pathIdPair{id: id, path: localPath}:
			return nil
		}
	default:
		return fmt.Errorf("unsupported file type: %v", fi.Mode.Type())
	}
	return nil
}

func receiveFile(r io.Reader, fi FileInfo, path string, buff []byte) error {
	var f *os.File
	var err error
	_, err = io.ReadFull(r, buff[:8])
	if err != nil {
		return fmt.Errorf("cannot read file size: %w", err)
	}
	size := int64(binary.BigEndian.Uint64(buff[:8]))
	var perm fs.FileMode = 0644
	if fi.Mode.Perm()&0111 != 0 {
		perm = 0755
	}
	f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("cannot create file for download: %w", err)
	}
	defer f.Close()
	var n int64
	n, err = io.CopyBuffer(f, io.LimitReader(r, size), buff)
	_ = f.Close()
	if err != nil {
		_, _ = io.CopyBuffer(io.Discard, io.LimitReader(r, size-n), buff)
		return fmt.Errorf("cannot download the file %w", err)
	}
	err = os.Chtimes(path, time.Now(), fi.ModTime)
	if err != nil {
		return fmt.Errorf("cannot set modtime of downloaded file: %w", err)
	}
	return nil
}

func createSignature(ctx context.Context, id uint32, path string, signatures chan<- dataChunk) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriterSize(chunkWriter{id: id, ch: signatures, done: ctx.Done()}, 1024)
	_, err = librsync.Signature(f, w, blockLen, strongLen, magic)
	if err != nil {
		return fmt.Errorf("cannot create signature: %w", err)
	}
	err = w.Flush()
	if err != nil {
		return fmt.Errorf("cannot flush: %w", err)
	}
	signatures <- dataChunk{id: id, data: nil}

	return nil
}

func applyPatch(r *io.PipeReader, fileInfo FileInfo, path string) error {
	var err error
	var patched, orig *os.File
	parent := filepath.Dir(path)
	name := filepath.Base(path)

	patched, err = os.CreateTemp(parent, "."+name+".new.*")
	if err != nil {
		return fmt.Errorf("cannot create file: %w", err)
	}
	defer patched.Close()

	var mode fs.FileMode = 0644
	if fileInfo.Mode.Perm()&0111 != 0 {
		mode = 0755
	}
	err = os.Chmod(patched.Name(), mode)
	if err != nil {
		return fmt.Errorf("cannot chmod: %w", err)
	}

	orig, err = os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer orig.Close()

	err = librsync.Patch(orig, r, patched)
	if err != nil {
		return fmt.Errorf("cannot apply patch: %w", err)
	}
	orig.Close()
	patched.Close()

	err = os.Rename(patched.Name(), path)
	if err != nil {
		return fmt.Errorf("cannot rename file: %w", err)
	}
	err = os.Chtimes(path, time.Now(), fileInfo.ModTime)
	if err != nil {
		return fmt.Errorf("cannot set mod time: %w", err)
	}

	return nil
}

type pathIdPair struct {
	id   uint32
	path string
}

type readerIdPair struct {
	id uint32
	r  *io.PipeReader
}
