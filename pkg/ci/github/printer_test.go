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

		err := PrintConfiguration(WorkflowConfig{}, "go", w)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("registry secrets write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		opts := WorkflowConfig{RegistryLogin: true}

		err := PrintConfiguration(opts, "go", w)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("single secret write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 2, err: errWrite}
		opts := WorkflowConfig{RegistryLogin: false}

		err := PrintConfiguration(opts, "go", w)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("unsupported runtime fails", func(t *testing.T) {
		runtime := "ruby"
		expectedErr := fmt.Errorf("no builder support for runtime: %s", runtime)

		err := PrintConfiguration(WorkflowConfig{}, runtime, &bytes.Buffer{})

		assert.Error(t, err, expectedErr.Error())
	})
}

func TestPrintPostExportMessageFail(t *testing.T) {
	t.Run("with registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		opts := WorkflowConfig{RegistryLogin: true}

		err := PrintPostExportMessage(opts, w)

		assert.Error(t, err, errWrite.Error())
	})

	t.Run("without registry login write fails", func(t *testing.T) {
		w := &failWriter{failOnCall: 1, err: errWrite}
		opts := WorkflowConfig{RegistryLogin: false}

		err := PrintPostExportMessage(opts, w)

		assert.Error(t, err, errWrite.Error())
	})
}
