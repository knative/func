package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/boson-project/faas"
	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/logging"
)

type Builder struct {
	Verbose bool
	Tag     string
}

func NewBuilder(tag string) *Builder {
	return &Builder{Tag: tag}
}

var runtime2pack = map[string]string{
	"quarkus": "quay.io/boson/faas-quarkus-builder",
	"node":    "quay.io/boson/faas-nodejs-builder",
	"go":      "quay.io/boson/faas-go-builder",
}

func (builder *Builder) Build(path string) (image string, err error) {
	f, err := faas.NewFunction(path)
	if err != nil {
		return
	}

	runtime := f.Runtime
	packBuilder, ok := runtime2pack[runtime]
	if !ok {
		err = errors.New(fmt.Sprint("unsupported runtime: ", runtime))
		return
	}

	var logWriter io.Writer
	if builder.Verbose {
		logWriter = os.Stdout
	} else {
		logWriter = &bytes.Buffer{}
	}

	logger := logging.New(logWriter)
	packClient, err := pack.NewClient(pack.WithLogger(logger))
	if err != nil {
		return
	}

	packOpts := pack.BuildOptions{
		AppPath: path,
		Image:   builder.Tag,
		Builder: packBuilder,
	}

	err = packClient.Build(context.Background(), packOpts)
	if err != nil {
		if !builder.Verbose {
			err = fmt.Errorf("%v\noutput: %s\n", err, logWriter.(*bytes.Buffer).String())
		}
	}

	return builder.Tag, err
}
