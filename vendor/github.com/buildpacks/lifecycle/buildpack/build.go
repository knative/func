package buildpack

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack/layertypes"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string, defaultAction env.ActionType) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

type BuildConfig struct {
	AppDir      string
	PlatformDir string
	LayersDir   string
	Out         io.Writer
	Err         io.Writer
	Logger      Logger
}

type BuildResult struct {
	BOM         []BOMEntry
	Labels      []Label
	MetRequires []string
	Processes   []launch.Process
	Slices      []layers.Slice
}

func (bom *BOMEntry) ConvertMetadataToVersion() {
	if version, ok := bom.Metadata["version"]; ok {
		metadataVersion := fmt.Sprintf("%v", version)
		bom.Version = metadataVersion
	}
}

func (bom *BOMEntry) convertVersionToMetadata() {
	if bom.Version != "" {
		if bom.Metadata == nil {
			bom.Metadata = make(map[string]interface{})
		}
		bom.Metadata["version"] = bom.Version
		bom.Version = ""
	}
}

func (b *Descriptor) Build(bpPlan Plan, config BuildConfig, bpEnv BuildEnv) (BuildResult, error) {
	config.Logger.Debugf("Running build for buildpack %s", b)

	if api.MustParse(b.API).Equal(api.MustParse("0.2")) {
		config.Logger.Debug("Updating buildpack plan entries")

		for i := range bpPlan.Entries {
			bpPlan.Entries[i].convertMetadataToVersion()
		}
	}

	config.Logger.Debug("Creating plan directory")
	planDir, err := ioutil.TempDir("", launch.EscapeID(b.Buildpack.ID)+"-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(planDir)

	config.Logger.Debug("Preparing paths")
	bpLayersDir, bpPlanPath, err := preparePaths(b.Buildpack.ID, bpPlan, config.LayersDir, planDir)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Running build command")
	if err := b.runBuildCmd(bpLayersDir, bpPlanPath, config, bpEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Processing layers")
	pathToLayerMetadataFile, err := b.processLayers(bpLayersDir, config.Logger)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Updating environment")
	if err := b.setupEnv(pathToLayerMetadataFile, bpEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Reading output files")
	return b.readOutputFiles(bpLayersDir, bpPlanPath, bpPlan, config.Logger)
}

func renameLayerDirIfNeeded(layerMetadataFile layertypes.LayerMetadataFile, layerDir string) error {
	// rename <layers>/<layer> to <layers>/<layer>.ignore if buildpack API >= 0.6 and all of the types flags are set to false
	if !layerMetadataFile.Launch && !layerMetadataFile.Cache && !layerMetadataFile.Build {
		if err := os.Rename(layerDir, layerDir+".ignore"); err != nil {
			return err
		}
	}
	return nil
}

func (b *Descriptor) processLayers(layersDir string, logger Logger) (map[string]layertypes.LayerMetadataFile, error) {
	if api.MustParse(b.API).LessThan("0.6") {
		return eachDir(layersDir, b.API, func(path, buildpackAPI string) (layertypes.LayerMetadataFile, error) {
			layerMetadataFile, msg, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
			if err != nil {
				return layertypes.LayerMetadataFile{}, err
			}
			if msg != "" {
				logger.Warn(msg)
			}
			return layerMetadataFile, nil
		})
	}
	return eachDir(layersDir, b.API, func(path, buildpackAPI string) (layertypes.LayerMetadataFile, error) {
		layerMetadataFile, msg, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
		if err != nil {
			return layertypes.LayerMetadataFile{}, err
		}
		if msg != "" {
			return layertypes.LayerMetadataFile{}, errors.New(msg)
		}
		if err := renameLayerDirIfNeeded(layerMetadataFile, path); err != nil {
			return layertypes.LayerMetadataFile{}, err
		}
		return layerMetadataFile, nil
	})
}

func preparePaths(bpID string, bpPlan Plan, layersDir, planDir string) (string, string, error) {
	bpDirName := launch.EscapeID(bpID)
	bpLayersDir := filepath.Join(layersDir, bpDirName)
	bpPlanDir := filepath.Join(planDir, bpDirName)
	if err := os.MkdirAll(bpLayersDir, 0777); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
		return "", "", err
	}
	bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")
	if err := WriteTOML(bpPlanPath, bpPlan); err != nil {
		return "", "", err
	}

	return bpLayersDir, bpPlanPath, nil
}

func WriteTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(data)
}

func (b *Descriptor) runBuildCmd(bpLayersDir, bpPlanPath string, config BuildConfig, bpEnv BuildEnv) error {
	cmd := exec.Command(
		filepath.Join(b.Dir, "bin", "build"),
		bpLayersDir,
		config.PlatformDir,
		bpPlanPath,
	) // #nosec G204
	cmd.Dir = config.AppDir
	cmd.Stdout = config.Out
	cmd.Stderr = config.Err

	var err error
	if b.Buildpack.ClearEnv {
		cmd.Env = bpEnv.List()
	} else {
		cmd.Env, err = bpEnv.WithPlatform(config.PlatformDir)
		if err != nil {
			return err
		}
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+b.Dir)

	if err := cmd.Run(); err != nil {
		return NewLifecycleError(err, ErrTypeBuildpack)
	}
	return nil
}

func (b *Descriptor) setupEnv(pathToLayerMetadataFile map[string]layertypes.LayerMetadataFile, buildEnv BuildEnv) error {
	bpAPI := api.MustParse(b.API)
	for path, layerMetadataFile := range pathToLayerMetadataFile {
		if !layerMetadataFile.Build {
			continue
		}
		if err := buildEnv.AddRootDir(path); err != nil {
			return err
		}
		if err := buildEnv.AddEnvDir(filepath.Join(path, "env"), env.DefaultActionType(bpAPI)); err != nil {
			return err
		}
		if err := buildEnv.AddEnvDir(filepath.Join(path, "env.build"), env.DefaultActionType(bpAPI)); err != nil {
			return err
		}
	}
	return nil
}

func eachDir(dir, buildpackAPI string, fn func(path, api string) (layertypes.LayerMetadataFile, error)) (map[string]layertypes.LayerMetadataFile, error) {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]layertypes.LayerMetadataFile{}, nil
	} else if err != nil {
		return map[string]layertypes.LayerMetadataFile{}, err
	}
	pathToLayerMetadataFile := map[string]layertypes.LayerMetadataFile{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		path := filepath.Join(dir, f.Name())
		layerMetadataFile, err := fn(path, buildpackAPI)
		if err != nil {
			return map[string]layertypes.LayerMetadataFile{}, err
		}
		pathToLayerMetadataFile[path] = layerMetadataFile
	}
	return pathToLayerMetadataFile, nil
}

func (b *Descriptor) readOutputFiles(bpLayersDir, bpPlanPath string, bpPlanIn Plan, logger Logger) (BuildResult, error) {
	br := BuildResult{}
	bpFromBpInfo := GroupBuildpack{ID: b.Buildpack.ID, Version: b.Buildpack.Version}

	// setup launch.toml
	var launchTOML LaunchTOML
	launchPath := filepath.Join(bpLayersDir, "launch.toml")

	if api.MustParse(b.API).LessThan("0.5") {
		// read buildpack plan
		var bpPlanOut Plan
		if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
			return BuildResult{}, err
		}

		// set BOM and MetRequires
		if err := validateBOM(bpPlanOut.toBOM(), b.API); err != nil {
			return BuildResult{}, err
		}
		br.BOM = WithBuildpack(bpFromBpInfo, bpPlanOut.toBOM())
		for i := range br.BOM {
			br.BOM[i].convertVersionToMetadata()
		}
		br.MetRequires = names(bpPlanOut.Entries)

		// read launch.toml, return if not exists
		if _, err := toml.DecodeFile(launchPath, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}
	} else {
		// read build.toml
		var bpBuild BuildTOML
		buildPath := filepath.Join(bpLayersDir, "build.toml")
		if _, err := toml.DecodeFile(buildPath, &bpBuild); err != nil && !os.IsNotExist(err) {
			return BuildResult{}, err
		}
		if err := validateBOM(bpBuild.BOM, b.API); err != nil {
			return BuildResult{}, err
		}

		// set MetRequires
		if err := validateUnmet(bpBuild.Unmet, bpPlanIn); err != nil {
			return BuildResult{}, err
		}
		br.MetRequires = names(bpPlanIn.filter(bpBuild.Unmet).Entries)

		// read launch.toml, return if not exists
		if _, err := toml.DecodeFile(launchPath, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}

		// set BOM
		if err := validateBOM(launchTOML.BOM, b.API); err != nil {
			return BuildResult{}, err
		}
		br.BOM = WithBuildpack(bpFromBpInfo, launchTOML.BOM)
	}

	if err := overrideDefaultForOldBuildpacks(launchTOML.Processes, b.API, logger); err != nil {
		return BuildResult{}, err
	}

	if err := validateNoMultipleDefaults(launchTOML.Processes); err != nil {
		return BuildResult{}, err
	}

	// set data from launch.toml
	br.Labels = append([]Label{}, launchTOML.Labels...)
	for i := range launchTOML.Processes {
		launchTOML.Processes[i].BuildpackID = b.Buildpack.ID
	}
	br.Processes = append([]launch.Process{}, launchTOML.Processes...)
	br.Slices = append([]layers.Slice{}, launchTOML.Slices...)

	return br, nil
}

func overrideDefaultForOldBuildpacks(processes []launch.Process, bpAPI string, logger Logger) error {
	if api.MustParse(bpAPI).AtLeast("0.6") {
		return nil
	}
	replacedDefaults := []string{}
	for i := range processes {
		if processes[i].Default {
			replacedDefaults = append(replacedDefaults, processes[i].Type)
		}
		processes[i].Default = false
	}
	if len(replacedDefaults) > 0 {
		logger.Warn(fmt.Sprintf("Warning: default processes aren't supported in this buildpack api version. Overriding the default value to false for the following processes: [%s]", strings.Join(replacedDefaults, ", ")))
	}
	return nil
}

func validateNoMultipleDefaults(processes []launch.Process) error {
	defaultType := ""
	for _, process := range processes {
		if process.Default && defaultType != "" {
			return fmt.Errorf("multiple default process types aren't allowed")
		}
		if process.Default {
			defaultType = process.Type
		}
	}
	return nil
}

func validateBOM(bom []BOMEntry, bpAPI string) error {
	if api.MustParse(bpAPI).LessThan("0.5") {
		for _, entry := range bom {
			if version, ok := entry.Metadata["version"]; ok {
				metadataVersion := fmt.Sprintf("%v", version)
				if entry.Version != "" && entry.Version != metadataVersion {
					return errors.New("top level version does not match metadata version")
				}
			}
		}
	} else {
		for _, entry := range bom {
			if entry.Version != "" {
				return fmt.Errorf("bom entry '%s' has a top level version which is not allowed. The buildpack should instead set metadata.version", entry.Name)
			}
		}
	}
	return nil
}

func validateUnmet(unmet []Unmet, bpPlan Plan) error {
	for _, unmet := range unmet {
		if unmet.Name == "" {
			return errors.New("unmet.name is required")
		}
		found := false
		for _, req := range bpPlan.Entries {
			if unmet.Name == req.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unmet.name '%s' must match a requested dependency", unmet.Name)
		}
	}
	return nil
}

func names(requires []Require) []string {
	var out []string
	for _, req := range requires {
		out = append(out, req.Name)
	}
	return out
}

func WithBuildpack(bp GroupBuildpack, bom []BOMEntry) []BOMEntry {
	var out []BOMEntry
	for _, entry := range bom {
		entry.Buildpack = bp.NoAPI().NoHomepage()
		out = append(out, entry)
	}
	return out
}
