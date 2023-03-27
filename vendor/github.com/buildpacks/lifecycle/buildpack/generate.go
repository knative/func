package buildpack

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
)

const (
	// EnvOutputDir is the absolute path of the extension output directory (read-write); a different copy is provided for each extension;
	// contents are copied to the generator's <generated> directory
	EnvOutputDir = "CNB_OUTPUT_DIR"
	// Also provided during generate: EnvExtensionDir (see detect.go); EnvBpPlanPath, EnvPlatformDir (see build.go)
)

type GenerateInputs struct {
	AppDir         string
	BuildConfigDir string
	OutputDir      string // a temp directory provided by the lifecycle to capture extensions output
	PlatformDir    string
	Env            BuildEnv
	Out, Err       io.Writer
	Plan           Plan
}

type GenerateOutputs struct {
	Dockerfiles []DockerfileInfo
	MetRequires []string
}

//go:generate mockgen -package testmock -destination ../testmock/generate_executor.go github.com/buildpacks/lifecycle/buildpack GenerateExecutor
type GenerateExecutor interface {
	Generate(d ExtDescriptor, inputs GenerateInputs, logger log.Logger) (GenerateOutputs, error)
}

type DefaultGenerateExecutor struct{}

func (e *DefaultGenerateExecutor) Generate(d ExtDescriptor, inputs GenerateInputs, logger log.Logger) (GenerateOutputs, error) {
	logger.Debug("Creating plan directory")
	planDir, err := os.MkdirTemp("", launch.EscapeID(d.Extension.ID)+"-")
	if err != nil {
		return GenerateOutputs{}, err
	}
	defer os.RemoveAll(planDir)

	logger.Debug("Preparing paths")
	extOutputDir, planPath, err := prepareInputPaths(d.Extension.ID, inputs.Plan, inputs.OutputDir, planDir)
	if err != nil {
		return GenerateOutputs{}, err
	}

	logger.Debug("Running generate command")
	if _, err = os.Stat(filepath.Join(d.WithRootDir, "bin", "generate")); err != nil {
		if os.IsNotExist(err) {
			// treat extension root directory as pre-populated output directory
			return readOutputFilesExt(d, filepath.Join(d.WithRootDir, "generate"), inputs.Plan, logger)
		}
		return GenerateOutputs{}, err
	}
	if err = runGenerateCmd(d, extOutputDir, planPath, inputs); err != nil {
		return GenerateOutputs{}, err
	}

	logger.Debug("Reading output files")
	return readOutputFilesExt(d, extOutputDir, inputs.Plan, logger)
}

func runGenerateCmd(d ExtDescriptor, extOutputDir, planPath string, inputs GenerateInputs) error {
	cmd := exec.Command(
		filepath.Join(d.WithRootDir, "bin", "generate"),
		extOutputDir,
		inputs.PlatformDir,
		planPath,
	) // #nosec G204
	cmd.Dir = inputs.AppDir
	cmd.Stdout = inputs.Out
	cmd.Stderr = inputs.Err

	var err error
	if d.Extension.ClearEnv {
		cmd.Env, err = inputs.Env.WithOverrides("", inputs.BuildConfigDir)
	} else {
		cmd.Env, err = inputs.Env.WithOverrides(inputs.PlatformDir, inputs.BuildConfigDir)
	}
	if err != nil {
		return err
	}
	cmd.Env = append(cmd.Env,
		EnvBpPlanPath+"="+planPath,
		EnvExtensionDir+"="+d.WithRootDir,
		EnvOutputDir+"="+extOutputDir,
		EnvPlatformDir+"="+inputs.PlatformDir,
	)

	if err := cmd.Run(); err != nil {
		return NewError(err, ErrTypeBuildpack)
	}
	return nil
}

func readOutputFilesExt(d ExtDescriptor, extOutputDir string, extPlanIn Plan, logger log.Logger) (GenerateOutputs, error) {
	br := GenerateOutputs{}
	var err error
	var dfInfo DockerfileInfo
	var found bool

	// set MetRequires
	br.MetRequires = names(extPlanIn.Entries)

	// set Dockerfiles
	if dfInfo, found, err = addDockerfileByPathAndType(d, extOutputDir, "run.Dockerfile", DockerfileKindRun, logger); err != nil {
		return GenerateOutputs{}, err
	} else if found {
		br.Dockerfiles = append(br.Dockerfiles, dfInfo)
	}

	if dfInfo, found, err = addDockerfileByPathAndType(d, extOutputDir, "build.Dockerfile", DockerfileKindBuild, logger); err != nil {
		return GenerateOutputs{}, err
	} else if found {
		br.Dockerfiles = append(br.Dockerfiles, dfInfo)
	}

	logger.Debugf("Found '%d' Dockerfiles for processing", len(br.Dockerfiles))

	return br, nil
}

func addDockerfileByPathAndType(d ExtDescriptor, extOutputDir string, dockerfileName string, dockerfileType string, _ log.Logger) (DockerfileInfo, bool, error) {
	var err error
	dockerfile := filepath.Join(extOutputDir, dockerfileName)
	if _, err = os.Stat(dockerfile); err != nil {
		// ignore file not found, no dockerfile to add.
		if !os.IsNotExist(err) {
			// any other errors are critical.
			return DockerfileInfo{}, true, err
		}
		return DockerfileInfo{}, false, nil
	}
	return DockerfileInfo{ExtensionID: d.Extension.ID, Kind: dockerfileType, Path: dockerfile}, true, nil
}
