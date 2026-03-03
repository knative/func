package ci

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"
)

var errWrite = errors.New("write failed")

type failWriter struct {
	failOnCall int
	callCount  int
	err        error
}

func (fw *failWriter) Write(p []byte) (int, error) {
	fw.callCount++
	if fw.callCount >= fw.failOnCall {
		return 0, fw.err
	}
	return len(p), nil
}

func TestPrintConfigurationFail(t *testing.T) {
	t.Run("main layout write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		conf := CIConfig{}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("registry secrets write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		conf := CIConfig{registryLogin: true}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("single secret write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		conf := CIConfig{registryLogin: false}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})
}

func TestPrintPostExportMessageFail(t *testing.T) {
	t.Run("with registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		conf := CIConfig{registryLogin: true}

		err := PrintPostExportMessage(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("without registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		conf := CIConfig{registryLogin: false}

		err := PrintPostExportMessage(w, conf)

		assert.Error(t, err, errWrite.Error())
	})
}
