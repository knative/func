package rsync

import (
	"encoding/binary"
	"io"
	"io/fs"
	"time"
)

type FileInfo struct {
	Path    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	Link    string
}

//region Wire

func writeByteArray(w io.Writer, bs []byte) error {
	var err error
	var buff [4]byte
	binary.BigEndian.PutUint32(buff[:4], uint32(len(bs)))
	_, err = w.Write(buff[:4])
	if err != nil {
		return err
	}
	if len(bs) > 0 {
		_, err = w.Write(bs)
		if err != nil {
			return err
		}
	}
	return nil
}

func readByteArray(r io.Reader, outBuff []byte) ([]byte, error) {
	var err error

	var buff [4]byte
	var size uint32

	_, err = io.ReadFull(r, buff[:4])
	if err != nil {
		return nil, err
	}
	size = binary.BigEndian.Uint32(buff[:4])

	if size > 0 {
		if len(outBuff) < int(size) {
			outBuff = make([]byte, size)
		} else {
			outBuff = outBuff[:size]
		}
		_, err = io.ReadFull(r, outBuff)
		if err != nil {
			return nil, err
		}
		return outBuff, nil
	}

	return nil, nil
}

func writeString(w io.Writer, s string) error {
	return writeByteArray(w, []byte(s))
}

func readString(r io.Reader) (string, error) {
	bs, err := readByteArray(r, nil)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func writeFileInfo(w io.Writer, f FileInfo) error {
	var err error
	var buff [24]byte

	err = writeString(w, f.Path)
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint64(buff[:], uint64(f.Size))
	binary.BigEndian.PutUint32(buff[8:], uint32(f.Mode))
	binary.BigEndian.PutUint64(buff[12:], uint64(f.ModTime.Unix()))
	binary.BigEndian.PutUint32(buff[20:], uint32(f.ModTime.Nanosecond()))
	_, err = w.Write(buff[:24])
	if err != nil {
		return err
	}

	if f.Mode.Type()&fs.ModeSymlink != 0 {
		err = writeString(w, f.Link)
		if err != nil {
			return err
		}
	}

	return nil
}

func readFileInfo(r io.Reader) (FileInfo, error) {
	var err error
	var f FileInfo
	var buff [24]byte

	f.Path, err = readString(r)
	if err != nil {
		return FileInfo{}, err
	}

	_, err = io.ReadFull(r, buff[:24])
	if err != nil {
		return FileInfo{}, err
	}

	f.Size = int64(binary.BigEndian.Uint64(buff[:]))
	f.Mode = fs.FileMode(binary.BigEndian.Uint32(buff[8:]))
	sec := int64(binary.BigEndian.Uint64(buff[12:]))
	nsec := int64(binary.BigEndian.Uint32(buff[20:]))

	f.ModTime = time.Unix(sec, nsec)

	if f.Mode.Type()&fs.ModeSymlink != 0 {
		f.Link, err = readString(r)
		if err != nil {
			return FileInfo{}, err
		}
	}

	return f, nil
}

//endregion Wire
