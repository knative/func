package rsync

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"

	v1 "knative.dev/func/rsync/v1"
)

var RemoteError = errors.New("remote error")

type ProcessFile = func(path, wpath string, fi fs.FileInfo, err error) error
type ForEachFile = func(processFile ProcessFile) error

const latestVersion = uint16(1)

func ReceiveFiles(conn io.ReadWriteCloser, root string) error {
	var buff [10]byte
	_, err := conn.Read(buff[:])
	if err != nil {
		return fmt.Errorf("cannot read preamble: %w", err)
	}
	if string(buff[:8]) != "funcsync" {
		return fmt.Errorf("invalid preamble")
	}

	ver := binary.BigEndian.Uint16(buff[8:])
	if ver > latestVersion {
		binary.BigEndian.PutUint16(buff[8:], latestVersion)
		ver = latestVersion
	}

	_, err = conn.Write(buff[:])
	if err != nil {
		return fmt.Errorf("cannot reply to preamble: %w", err)
	}

	switch ver {
	case 1:
		return v1.ReceiveFiles(conn, root)
	default:
		return fmt.Errorf("unsupported protocol version")
	}
}

func SendFiles(conn io.ReadWriteCloser, forEachFile ForEachFile) error {
	var buff = []byte("funcsyncXX")
	binary.BigEndian.PutUint16(buff[8:], latestVersion)

	_, err := conn.Write(buff)
	if err != nil {
		return fmt.Errorf("cannot write preamble: %w", err)
	}

	_, err = conn.Read(buff[:])
	if err != nil {
		return fmt.Errorf("cannot read preamble: %w", err)
	}
	if string(buff[:8]) != "funcsync" {
		return fmt.Errorf("invalid preamble response")
	}

	ver := binary.BigEndian.Uint16(buff[8:])
	switch ver {
	case 1:
		return v1.SendFiles(conn, forEachFile)
	default:
		return fmt.Errorf("unsupported protocol version")
	}

}
