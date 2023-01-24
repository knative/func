package v1

import (
	"context"
	"runtime"

	"github.com/balena-os/librsync-go"
)

type messageType = byte

const (
	signatureData messageType = iota + 128
	deltaData
	fileData
	endOfExchange
	errorData
)

const fileListEndSentinel = "\000\000\000\000"

var workerCount = runtime.GOMAXPROCS(-1) + 1

const chanBuffSize = 0
const blockLen = 2 * (1 << 10)
const strongLen = 32
const magic = librsync.BLAKE2_SIG_MAGIC

type dataChunk struct {
	id   uint32
	data []byte
}

type chunkWriter struct {
	id   uint32
	ch   chan<- dataChunk
	done <-chan struct{}
}

func (s chunkWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case <-s.done:
		return 0, context.Canceled
	case s.ch <- dataChunk{id: s.id, data: data}:
		return len(p), nil
	}
}
