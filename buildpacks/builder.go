package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/logging"
	"io"
	"os"
)

type Builder struct {
	Verbose   bool
	registry  string
	namespace string
}

func NewBuilder(registry, namespace string) *Builder {
	return &Builder{registry: registry, namespace: namespace}
}

var lang2pack = map[string]string{
	"java": "quay.io/boson/faas-quarkus-builder",
	"js":   "quay.io/boson/faas-nodejs-builder",
}

func (builder *Builder) Build(name, language, path string) (image string, err error) {

	registry := fmt.Sprintf("%s/%s", builder.registry, builder.namespace)
	image = fmt.Sprintf("%s/%s", registry, name)
	packBuilder, ok := lang2pack[language]
	if !ok {
		err = errors.New(fmt.Sprint("unsupported language: ", language))
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
