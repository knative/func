package platform

import (
	"path/filepath"

	"github.com/buildpacks/lifecycle/api"
)

const (
	DefaultAnalyzedFile = "analyzed.toml"
	DefaultGroupFile    = "group.toml"
	// TODO: future work should move order, plan, project metadata, and report to this file
)

var (
	PlaceholderAnalyzedPath = filepath.Join("<layers>", DefaultAnalyzedFile)
	PlaceholderGroupPath    = filepath.Join("<layers>", DefaultGroupFile)
)

func defaultPath(placeholderPath, layersDir string, platformAPI *api.Version) string {
	filename := filepath.Base(placeholderPath)
	if (platformAPI).LessThan("0.5") || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", filename)
	}
	return filepath.Join(layersDir, filename)
}
