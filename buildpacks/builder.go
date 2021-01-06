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

	bosonFunc "github.com/boson-project/func"
)

//Builder holds the configuration that will be passed to
//Buildpack builder
type Builder struct {
	Verbose bool
}

//NewBuilder builds the new Builder configuration
func NewBuilder() *Builder {
	return &Builder{}
}

//RuntimeToBuildpack holds the mapping between the Runtime and its corresponding
//Buildpack builder to use
var RuntimeToBuildpack = map[string]string{
	"quarkus":    "quay.io/boson/faas-quarkus-builder",
	"node":       "quay.io/boson/faas-nodejs-builder",
	"go":         "quay.io/boson/faas-go-builder",
	"springboot": "quay.io/boson/faas-springboot-builder",
}

// Build the Function at path.
func (builder *Builder) Build(f bosonFunc.Function) (err error) {

	// Use the builder found in the Function configuration file
	// If one isn't found, use the defaults
	var packBuilder string
	if f.Builder != "" {
		packBuilder = f.Builder
		pb, ok := f.BuilderMap[packBuilder]
		if ok {
			packBuilder = pb
		}
	} else {
		packBuilder = RuntimeToBuildpack[f.Runtime]
		if packBuilder == "" {
			return errors.New(fmt.Sprint("unsupported runtime: ", f.Runtime))
		}
	}

	// Build options for the pack client.
	packOpts := pack.BuildOptions{
		AppPath: f.Root,
		Image:   f.Image,
		Builder: packBuilder,
	}

	// log output is either STDOUt or kept in a buffer to be printed on error.
	var logWriter io.Writer
	if builder.Verbose {
		logWriter = os.Stdout
	} else {
		logWriter = &bytes.Buffer{}
	}

	// Client with a logger which is enabled if in Verbose mode.
	packClient, err := pack.NewClient(pack.WithLogger(logging.New(logWriter)))
	if err != nil {
		return
	}

	// Build based using the given builder.
	if err = packClient.Build(context.Background(), packOpts); err != nil {
		// If the builder was not showing logs, embed the full logs in the error.
		if !builder.Verbose {
			err = fmt.Errorf("%v\noutput: %s\n", err, logWriter.(*bytes.Buffer).String())
		}
	}

	return
}
