package github

import (
	"bytes"
	"errors"
	"fmt"
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
		conf := Config{FnRuntime: "go"}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("registry secrets write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		conf := Config{RegistryLogin: true, FnRuntime: "go"}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("single secret write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		conf := Config{RegistryLogin: false, FnRuntime: "go"}

		err := PrintConfiguration(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("unsupported runtime fails", func(t *testing.T) {
		conf := Config{FnRuntime: "ruby"}
		expectedErr := fmt.Errorf("no builder support for runtime: %s", conf.FnRuntime)

		err := PrintConfiguration(&bytes.Buffer{}, conf)

		assert.Error(t, err, expectedErr.Error())
	})
}

func TestPrintPostExportMessageFail(t *testing.T) {
	t.Run("with registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		conf := Config{RegistryLogin: true}

		err := PrintPostExportMessage(w, conf)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("without registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		conf := Config{RegistryLogin: false}

		err := PrintPostExportMessage(w, conf)

		assert.Error(t, err, errWrite.Error())
	})
}
