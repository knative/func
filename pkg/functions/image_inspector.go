package functions

import (
	"net/http"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type ImageInspector struct {
	transport http.RoundTripper
}

func NewImageInspector(transport http.RoundTripper) *ImageInspector {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &ImageInspector{transport: transport}
}

func (i *ImageInspector) Labels(image string) (map[string]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}

	desc, err := remote.Get(ref, remote.WithTransport(i.transport))
	if err != nil {
		return nil, err
	}

	img, err := desc.Image()
	if err != nil {
		return nil, err
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	return cfg.Config.Labels, nil
}

func (i *ImageInspector) MiddlewareVersion(image string) (string, error) {
	labels, err := i.Labels(image)
	if err != nil {
		return "", err
	}
	if labels == nil {
		return "", nil
	}
	return labels[MiddlewareVersionLabelKey], nil
}

func (i *ImageInspector) Commit(image string) (string, error) {
	labels, err := i.Labels(image)
	if err != nil {
		return "", err
	}
	if labels == nil {
		return "", nil
	}
	return labels[CommitLabelKey], nil
}
