package lifecycle

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/launch"
)

type GroupBuildpack struct {
	ID       string `toml:"id" json:"id"`
	Version  string `toml:"version" json:"version"`
	Optional bool   `toml:"optional,omitempty" json:"optional,omitempty"`
	API      string `toml:"api,omitempty" json:"-"`
	Homepage string `toml:"homepage,omitempty" json:"homepage,omitempty"`
}

func (bp GroupBuildpack) String() string {
	return bp.ID + "@" + bp.Version
}

func (bp GroupBuildpack) noOpt() GroupBuildpack {
	bp.Optional = false
	return bp
}

func (bp GroupBuildpack) noAPI() GroupBuildpack {
	bp.API = ""
	return bp
}

func (bp GroupBuildpack) noHomepage() GroupBuildpack {
	bp.Homepage = ""
	return bp
}

func (bp GroupBuildpack) Lookup(buildpacksDir string) (*BuildpackTOML, error) {
	bpTOML := BuildpackTOML{}
	bpPath, err := filepath.Abs(filepath.Join(buildpacksDir, launch.EscapeID(bp.ID), bp.Version))
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = bpPath
	return &bpTOML, nil
}

type BuildpackInfo struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Name     string `toml:"name"`
	ClearEnv bool   `toml:"clear-env,omitempty"`
	Homepage string `toml:"homepage,omitempty"`
}
