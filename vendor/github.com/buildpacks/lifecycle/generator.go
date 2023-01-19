package lifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

type Generator struct {
	AppDir       string
	GeneratedDir string // e.g., <layers>/generated
	PlatformDir  string
	DirStore     DirStore
	Executor     buildpack.GenerateExecutor
	Extensions   []buildpack.GroupElement
	Logger       log.Logger
	Out, Err     io.Writer
	Plan         platform.BuildPlan
}

type GeneratorFactory struct {
	apiVerifier BuildpackAPIVerifier
	dirStore    DirStore
}

func NewGeneratorFactory(
	apiVerifier BuildpackAPIVerifier,
	dirStore DirStore,
) *GeneratorFactory {
	return &GeneratorFactory{
		apiVerifier: apiVerifier,
		dirStore:    dirStore,
	}
}

func (f *GeneratorFactory) NewGenerator(
	appDir string,
	extensions []buildpack.GroupElement,
	generatedDir string,
	plan platform.BuildPlan,
	platformDir string,
	stdout, stderr io.Writer,
	logger log.Logger,
) (*Generator, error) {
	generator := &Generator{
		AppDir:       appDir,
		GeneratedDir: generatedDir,
		PlatformDir:  platformDir,
		DirStore:     f.dirStore,
		Executor:     &buildpack.DefaultGenerateExecutor{},
		Logger:       logger,
		Plan:         plan,
		Out:          stdout,
		Err:          stderr,
	}

	if err := f.setExtensions(generator, extensions, logger); err != nil {
		return nil, err
	}
	return generator, nil
}

func (f *GeneratorFactory) setExtensions(generator *Generator, extensions []buildpack.GroupElement, logger log.Logger) error {
	generator.Extensions = extensions
	for _, el := range generator.Extensions {
		if err := f.apiVerifier.VerifyBuildpackAPI(buildpack.KindExtension, el.String(), el.API, logger); err != nil {
			return err
		}
	}
	return nil
}

type GenerateResult struct {
	RunImage string
	Plan     platform.BuildPlan
	UsePlan  bool
}

func (g *Generator) Generate() (GenerateResult, error) {
	inputs := g.getCommonInputs()
	extensionOutputParentDir, err := os.MkdirTemp("", "cnb-extensions-generated.")
	if err != nil {
		return GenerateResult{}, err
	}
	defer os.RemoveAll(extensionOutputParentDir)
	inputs.OutputDir = extensionOutputParentDir

	var dockerfiles []buildpack.DockerfileInfo
	filteredPlan := g.Plan
	for _, ext := range g.Extensions {
		g.Logger.Debugf("Running generate for extension %s", ext)

		g.Logger.Debug("Looking up extension")
		descriptor, err := g.DirStore.LookupExt(ext.ID, ext.Version)
		if err != nil {
			return GenerateResult{}, err
		}

		g.Logger.Debug("Finding plan")
		inputs.Plan = filteredPlan.Find(buildpack.KindExtension, ext.ID)

		g.Logger.Debug("Invoking command")
		result, err := g.Executor.Generate(*descriptor, inputs, g.Logger)
		if err != nil {
			return GenerateResult{}, err
		}

		// aggregate build results
		dockerfiles = append(dockerfiles, result.Dockerfiles...)
		filteredPlan = filteredPlan.Filter(result.MetRequires)

		g.Logger.Debugf("Finished running generate for extension %s", ext)
	}

	g.Logger.Debug("Validating Dockerfiles")
	if err := g.validateDockerfiles(dockerfiles); err != nil {
		return GenerateResult{}, err
	}

	g.Logger.Debug("Copying Dockerfiles")
	if err := g.copyDockerfiles(dockerfiles); err != nil {
		return GenerateResult{}, err
	}

	g.Logger.Debug("Checking for new run image")
	runImage, err := g.checkNewRunImage()
	if err != nil {
		return GenerateResult{}, err
	}

	g.Logger.Debugf("Finished build, selected runImage '%s'", runImage)
	return GenerateResult{Plan: filteredPlan, UsePlan: true, RunImage: runImage}, nil
}

func (g *Generator) getCommonInputs() buildpack.GenerateInputs {
	return buildpack.GenerateInputs{
		AppDir:      g.AppDir,
		PlatformDir: g.PlatformDir,
		Env:         env.NewBuildEnv(os.Environ()),
		Out:         g.Out,
		Err:         g.Err,
	}
}

func (g *Generator) validateDockerfiles(dockerfiles []buildpack.DockerfileInfo) error {
	for _, dockerfile := range dockerfiles {
		switch {
		case dockerfile.Kind == buildpack.DockerfileKindRun:
			if err := buildpack.VerifyRunDockerfile(dockerfile.Path); err != nil {
				return fmt.Errorf("error parsing run.Dockerfile for extension %s: %w", dockerfile.ExtensionID, err)
			}
		case dockerfile.Kind == buildpack.DockerfileKindBuild:
			if err := buildpack.VerifyBuildDockerfile(dockerfile.Path, g.Logger); err != nil {
				return fmt.Errorf("error parsing build.Dockerfile for extension %s: %w", dockerfile.ExtensionID, err)
			}
		}
	}
	return nil
}

func (g *Generator) copyDockerfiles(dockerfiles []buildpack.DockerfileInfo) error {
	for _, dockerfile := range dockerfiles {
		targetDir := filepath.Join(g.GeneratedDir, dockerfile.Kind, launch.EscapeID(dockerfile.ExtensionID))
		targetPath := filepath.Join(targetDir, "Dockerfile")
		if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
			return err
		}
		if err := fsutil.Copy(dockerfile.Path, targetPath); err != nil {
			return err
		}
		// check for extend-config.toml and if found, copy
		extendConfigPath := filepath.Join(filepath.Dir(dockerfile.Path), "extend-config.toml")
		if err := fsutil.Copy(extendConfigPath, filepath.Join(targetDir, "extend-config.toml")); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (g *Generator) checkNewRunImage() (string, error) {
	// There may be extensions that contribute only a build.Dockerfile; work backward through extensions until we find
	// a run.Dockerfile.
	for i := len(g.Extensions) - 1; i >= 0; i-- {
		extID := g.Extensions[i].ID
		runDockerfile := filepath.Join(g.GeneratedDir, "run", launch.EscapeID(extID), "Dockerfile")
		if _, err := os.Stat(runDockerfile); os.IsNotExist(err) {
			continue
		}

		imageName, err := buildpack.RetrieveFirstFromImageNameFromDockerfile(runDockerfile)
		if err != nil {
			return "", err
		}

		g.Logger.Debugf("Found a run.Dockerfile configuring image '%s' from extension with id '%s'", imageName, extID)
		return imageName, nil
	}
	return "", nil
}
