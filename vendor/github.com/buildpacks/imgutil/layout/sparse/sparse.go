package sparse

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"

	"github.com/buildpacks/imgutil/layout"

	"github.com/buildpacks/imgutil"
)

var _ imgutil.Image = (*Image)(nil)

// Image is a struct created to overrides the Save() method of the image.
// a sparse Image is saved on disk but does not include any layers in the `blobs` directory.
type Image struct {
	layout.Image
}

// NewImage returns a new Image saved on disk that can be modified
func NewImage(path string, from v1.Image) (*Image, error) {
	img, err := layout.NewImage(path, layout.FromBaseImage(from))
	if err != nil {
		return nil, err
	}

	image := &Image{
		Image: *img,
	}
	return image, nil
}

func (i *Image) Save(additionalNames ...string) error {
	return i.SaveAs(i.Name(), additionalNames...)
}

func (i *Image) SaveAs(name string, additionalNames ...string) error {
	var diagnostics []imgutil.SaveDiagnostic

	refName, _ := i.Image.GetAnnotateRefName()
	annotations := layout.ImageRefAnnotation(refName)

	pathsToSave := append([]string{name}, additionalNames...)
	for _, path := range pathsToSave {
		layoutPath, err := layout.Write(path, empty.Index)
		if err != nil {
			return err
		}

		err = layoutPath.AppendImage(i, layout.WithoutLayers(), layout.WithAnnotations(annotations))
		if err != nil {
			diagnostics = append(diagnostics, imgutil.SaveDiagnostic{ImageName: name, Cause: err})
		}
	}

	if len(diagnostics) > 0 {
		return imgutil.SaveError{Errors: diagnostics}
	}

	return nil
}
