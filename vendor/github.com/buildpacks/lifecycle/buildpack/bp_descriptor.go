// Buildpack descriptor file (https://github.com/buildpacks/spec/blob/main/buildpack.md#buildpacktoml-toml).

package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type BpDescriptor struct {
	WithAPI     string `toml:"api"`
	Buildpack   BpInfo `toml:"buildpack"`
	Order       Order  `toml:"order"`
	WithRootDir string `toml:"-"`
}

type BpInfo struct {
	BaseInfo
	SBOM []string `toml:"sbom-formats,omitempty" json:"sbom-formats,omitempty"`
}

type Order []Group

type Group struct {
	Group           []GroupElement `toml:"group"`
	GroupExtensions []GroupElement `toml:"group-extensions,omitempty" json:"group-extensions,omitempty"`
}

func ReadBpDescriptor(path string) (*BpDescriptor, error) {
	var (
		descriptor *BpDescriptor
		err        error
	)
	if _, err = toml.DecodeFile(path, &descriptor); err != nil {
		return &BpDescriptor{}, err
	}
	if descriptor.WithRootDir, err = filepath.Abs(filepath.Dir(path)); err != nil {
		return &BpDescriptor{}, err
	}
	return descriptor, nil
}

func (d *BpDescriptor) API() string {
	return d.WithAPI
}

func (d *BpDescriptor) ClearEnv() bool {
	return d.Buildpack.ClearEnv
}

func (d *BpDescriptor) Homepage() string {
	return d.Buildpack.Homepage
}

func (d *BpDescriptor) RootDir() string {
	return d.WithRootDir
}

func (d *BpDescriptor) String() string {
	return d.Buildpack.Name + " " + d.Buildpack.Version
}

func (bg Group) Append(group ...Group) Group {
	for _, g := range group {
		bg.Group = append(bg.Group, g.Group...)
	}
	return bg
}

func (bg Group) HasExtensions() bool {
	return len(bg.GroupExtensions) > 0
}

// A GroupElement represents a buildpack referenced in a buildpack.toml's [[order.group]] OR
// a buildpack or extension in order.toml OR a buildpack or extension in group.toml.
type GroupElement struct {
	// Fields that are common to order.toml and group.toml

	// ID specifies the ID of the buildpack or extension.
	ID string `toml:"id" json:"id"`
	// Version specifies the version of the buildpack or extension.
	Version string `toml:"version" json:"version"`

	// Fields that are in group.toml only

	// API specifies the Buildpack API that the buildpack or extension implements.
	API string `toml:"api,omitempty" json:"-"`
	// Homepage specifies the homepage of the buildpack or extension.
	Homepage string `toml:"homepage,omitempty" json:"homepage,omitempty"`
	// Extension specifies whether the group element is a buildpack or an extension.
	Extension bool `toml:"extension,omitempty" json:"-"`

	// Fields that are in order.toml only

	// Optional specifies that the buildpack or extension is optional. Extensions are always optional.
	Optional bool `toml:"optional,omitempty" json:"optional,omitempty"`

	// Fields that are never written

	// OrderExtensions holds the order for extensions during the detect phase.
	OrderExtensions Order `toml:"-" json:"-"`
}

func (e GroupElement) Equals(o GroupElement) bool {
	return e.ID == o.ID &&
		e.Version == o.Version &&
		e.API == o.API &&
		e.Homepage == o.Homepage &&
		e.Extension == o.Extension &&
		e.Optional == o.Optional
}

func (e GroupElement) IsExtensionsOrder() bool {
	return len(e.OrderExtensions) > 0
}

func (e GroupElement) Kind() string {
	if e.Extension {
		return KindExtension
	}
	return KindBuildpack
}

func (e GroupElement) NoAPI() GroupElement {
	e.API = ""
	return e
}

func (e GroupElement) NoExtension() GroupElement {
	e.Extension = false
	return e
}

func (e GroupElement) NoHomepage() GroupElement {
	e.Homepage = ""
	return e
}

func (e GroupElement) NoOpt() GroupElement {
	e.Optional = false
	return e
}

func (e GroupElement) String() string {
	return e.ID + "@" + e.Version
}

func (e GroupElement) WithAPI(version string) GroupElement {
	e.API = version
	return e
}

func (e GroupElement) WithHomepage(address string) GroupElement {
	e.Homepage = address
	return e
}
