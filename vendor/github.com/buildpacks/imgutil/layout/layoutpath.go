package layout

import (
	"path/filepath"

	ggcr "github.com/google/go-containerregistry/pkg/v1/layout"
)

type Path struct {
	ggcr.Path
}

type Option func(*options)

type options struct {
	withoutLayers bool
	annotations   map[string]string
}

func WithoutLayers() Option {
	return func(i *options) {
		i.withoutLayers = true
	}
}

func WithAnnotations(annotations map[string]string) Option {
	return func(i *options) {
		i.annotations = annotations
	}
}

func (l Path) append(elem ...string) string {
	complete := []string{string(l.Path)}
	return filepath.Join(append(complete, elem...)...)
}
