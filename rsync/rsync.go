package rsync

import (
	"errors"
	"io"
	"io/fs"

	v1 "knative.dev/func/rsync/v1"
)

var RemoteError = errors.New("remote error")

type ProcessFile = func(path, wpath string, fi fs.FileInfo, err error) error
type ForEachFile = func(processFile ProcessFile) error

func ReceiveFiles(conn io.ReadWriteCloser, root string) error {
	return v1.ReceiveFiles(conn, root)
}

func SendFiles(conn io.ReadWriteCloser, forEachFile ForEachFile) error {
	return v1.SendFiles(conn, forEachFile)
}
