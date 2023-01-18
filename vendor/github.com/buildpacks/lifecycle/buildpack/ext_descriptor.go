// Buildpack descriptor file (https://github.com/buildpacks/spec/blob/main/buildpack.md#buildpacktoml-toml).

package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ExtDescriptor struct {
	WithAPI     string  `toml:"api"`
	Extension   ExtInfo `toml:"extension"`
	WithRootDir string  `toml:"-"`
}

type ExtInfo struct {
	BaseInfo
}

func ReadExtDescriptor(path string) (*ExtDescriptor, error) {
	var (
		descriptor *ExtDescriptor
		err        error
	)
	if _, err = toml.DecodeFile(path, &descriptor); err != nil {
		return &ExtDescriptor{}, err
	}
	if descriptor.WithRootDir, err = filepath.Abs(filepath.Dir(path)); err != nil {
		return &ExtDescriptor{}, err
	}
	return descriptor, nil
}

func (d *ExtDescriptor) API() string {
	return d.WithAPI
}

func (d *ExtDescriptor) ClearEnv() bool {
	return d.Extension.ClearEnv
}

func (d *ExtDescriptor) Homepage() string {
	return d.Extension.Homepage
}

func (d *ExtDescriptor) RootDir() string {
	return d.WithRootDir
}

func (d *ExtDescriptor) String() string {
	return d.Extension.Name + " " + d.Extension.Version
}
