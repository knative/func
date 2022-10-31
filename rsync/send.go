package rsync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync/atomic"

	"github.com/balena-os/librsync-go"

	"golang.org/x/sync/errgroup"
)

type ProcessFile = func(path, wpath string, fi fs.FileInfo, err error) error
type ForEachFile = func(processFile ProcessFile) error

func SendFiles(conn io.ReadWriteCloser, forEachFile ForEachFile) error {
	var err error
	var paths = make([]string, 0, 32<<10)
	var files = make([]FileInfo, 0, 32<<10)

	w := bufio.NewWriter(conn)
	//region Send file list
	err = forEachFile(func(path, wpath string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		f := FileInfo{
			Path:    wpath,
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			Link:    "",
		}

		switch {
		case fi.IsDir():
		case fi.Mode().Type()&fs.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("cannot read link: %w", err)
			}
			f.Link = linkTarget
		case fi.Mode().Type() == 0: // regular file
			f.Size = fi.Size()
		default:
			return fmt.Errorf("unsupported file type: %v", fi.Mode().Type())
		}

		paths = append(paths, path)
		files = append(files, f)

		err = writeFileInfo(w, f)
		if err != nil {
			return fmt.Errorf("cannot write file to output: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error while sending file list: %w", err)
	}
	err = writeFileInfo(w, FileInfo{Path: fileListEndSentinel})
	if err != nil {
		return fmt.Errorf("cannot send end of file list marker: %w", err)
	}
	err = w.Flush()
	if err != nil {
		return fmt.Errorf("cannot flush file list: %w", err)
	}
	//endregion Send file list

	eg, ctx := errgroup.WithContext(context.Background())
	signatures := make(chan signatureIdPair, chanBuffSize)
	requestedFiles := make(chan uint32, chanBuffSize)
	deltas := make(chan dataChunk, chanBuffSize)

	//region Generate deltas from signatures
	deltaGeneratorsCnt := int32(workerCount)
	for i := 0; i < workerCount; i++ {
		// delta generator
		eg.Go(func() error {
			var err error
			defer func() {
				cnt := atomic.AddInt32(&deltaGeneratorsCnt, -1)
				if cnt == 0 {
					close(deltas)
					for range signatures {
					}
				}
			}()
			litBuff := make([]byte, 0, librsync.OUTPUT_BUFFER_SIZE)
			for sb := range signatures {
				err = createDelta(ctx, sb.id, paths[sb.id], sb.sig, deltas, litBuff)
				if err != nil {
					return fmt.Errorf("cannot create delta: %w", err)
				}
			}
			return nil
		})
	}
	//endregion Generate deltas from signatures

	//region Handle incoming data
	eg.Go(func() error {
		defer func() {
			close(signatures)
			close(requestedFiles)
		}()
		var err error
		partialSignatures := make(map[uint32]*bytes.Buffer)
		var buff = make([]byte, 32*(1<<10))
		r := bufio.NewReader(conn)
		for {
			var msgType messageType
			msgType, err = r.ReadByte()
			if err != nil {
				return fmt.Errorf("cannot read message type from input: %w", err)
			}

			if msgType == endOfExchange {
				return nil
			}

			var id uint32
			_, err = io.ReadFull(r, buff[:4])
			if err != nil {
				return fmt.Errorf("cannot read file id: %w", err)
			}
			id = binary.BigEndian.Uint32(buff[:4])

			switch msgType {
			case fileData:
				select {
				case <-ctx.Done():
					continue
				case requestedFiles <- id:
				}
			case signatureData:
				data, err := readByteArray(r, buff)
				if err != nil {
					return fmt.Errorf("cannot signature chunk: %w", err)
				}

				select {
				case <-ctx.Done():
					continue
				default:
				}

				sigBuff, ok := partialSignatures[id]
				if !ok {
					sigBuff = &bytes.Buffer{}
					partialSignatures[id] = sigBuff
				}

				if len(data) > 0 {
					_, _ = sigBuff.Write(data)
				} else {
					select {
					case <-ctx.Done():
						continue
					case signatures <- signatureIdPair{id: id, sig: sigBuff}:
					}
					delete(partialSignatures, id)
				}
			default:
				panic(fmt.Sprintf("unknown message type: 0x%x\n", msgType))
			}
		}
	})
	//endregion Handle incoming data

	//region Handle outcoming data
	var buff = make([]byte, 32*(1<<10))
out:
	for {
		select {
		case id, ok := <-requestedFiles:
			if !ok {
				requestedFiles = nil
				break
			}
			err = sendFile(w, id, files[id], paths[id], buff)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "cannot send file: %v", err)
			}
		case p, ok := <-deltas:
			if !ok {
				deltas = nil
				break
			}
			err = sendDeltaChunk(w, p, buff)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "cannot send delta chunk: %v", err)
			}
		}

		if deltas == nil && requestedFiles == nil {
			err = w.WriteByte(endOfExchange)
			if err != nil {
				return fmt.Errorf("cannot write end of exchange: %w", err)
			}
			err = w.Flush()
			if err != nil {
				return fmt.Errorf("cannot flush data: %w", err)
			}
			break out
		}
	}
	//endregion Handle outcoming data

	err = eg.Wait()
	if err != nil {
		return err
	}
	return nil
}

func sendFile(w io.Writer, id uint32, fi FileInfo, path string, buff []byte) error {
	var f *os.File
	var err error

	f, err = os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file for upload: %w", err)
	}
	defer f.Close()

	buff[0] = fileData
	binary.BigEndian.PutUint32(buff[1:], id)
	binary.BigEndian.PutUint64(buff[5:], uint64(fi.Size))
	_, err = w.Write(buff[:13])
	if err != nil {
		return err
	}
	_, err = io.CopyBuffer(w, f, buff)
	if err != nil {
		return fmt.Errorf("cannot upload the file: %w", err)
	}
	return nil
}

func sendDeltaChunk(w io.Writer, p dataChunk, buff []byte) error {
	var err error
	buff[0] = deltaData
	binary.BigEndian.PutUint32(buff[1:], p.id)
	_, err = w.Write(buff[:5])
	if err != nil {
		return fmt.Errorf("cannot write file id: %w", err)
	}
	err = writeByteArray(w, p.data)
	if err != nil {
		return fmt.Errorf("cannot write delata: %w", err)
	}
	return nil
}

func createDelta(ctx context.Context, id uint32, path string, sigData *bytes.Buffer, deltas chan<- dataChunk, litBuff []byte) error {

	sig, err := librsync.ReadSignature(sigData)
	if err != nil {
		return fmt.Errorf("cannot deserialize signature: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriterSize(chunkWriter{id: id, ch: deltas, done: ctx.Done()}, 1024)
	err = librsync.DeltaBuff(sig, f, w, litBuff)
	if err != nil {
		return fmt.Errorf("cannot create delta: %w", err)
	}
	err = w.Flush()
	if err != nil {
		return fmt.Errorf("cannot flush: %w", err)
	}

	deltas <- dataChunk{id: id, data: nil}

	return nil
}

type signatureIdPair struct {
	id  uint32
	sig *bytes.Buffer
}
