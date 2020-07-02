package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/logging"
)

type Builder struct {
	Verbose   bool
	registry  string
	namespace string
}

func NewBuilder(registry, namespace string) *Builder {
	return &Builder{registry: registry, namespace: namespace}
}

var runtime2pack = map[string]string{
	"quarkus": "quay.io/boson/faas-quarkus-builder",
	"node":    "quay.io/boson/faas-nodejs-builder",
	"go":      "quay.io/boson/faas-go-builder",
}

func (builder *Builder) Build(name, runtime, path string) (image string, err error) {

	registry := fmt.Sprintf("%s/%s", builder.registry, builder.namespace)
	image = fmt.Sprintf("%s/%s", registry, name)
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
		AppPath:  path,
		Image:    image,
		Builder:  packBuilder,
		Registry: registry,
	}

	err = packClient.Build(context.Background(), packOpts)
	if err != nil {
		if !builder.Verbose {
			err = fmt.Errorf("%v\noutput: %s\n", err, logWriter.(*bytes.Buffer).String())
		}
	}

	return
}
