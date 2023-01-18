package buildpack

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/buildpacks/lifecycle/log"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

var permittedCommandsBuild = []string{"FROM", "ADD", "ARG", "COPY", "ENV", "LABEL", "RUN", "SHELL", "USER", "WORKDIR"}

type DockerfileInfo struct {
	ExtensionID string
	Kind        string
	Path        string
}

type ExtendConfig struct {
	Build ExtendBuildConfig `toml:"build"`
}

type ExtendBuildConfig struct {
	Args []ExtendArg `toml:"args"`
}

type ExtendArg struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

func parseDockerfile(dockerfile string) ([]instructions.Stage, []instructions.ArgCommand, error) {
	var err error
	var d []uint8
	d, err = os.ReadFile(dockerfile)
	if err != nil {
		return nil, nil, err
	}
	p, err := parser.Parse(bytes.NewReader(d))
	if err != nil {
		return nil, nil, err
	}
	stages, metaArgs, err := instructions.Parse(p.AST)
	if err != nil {
		return nil, nil, err
	}
	return stages, metaArgs, nil
}

func VerifyBuildDockerfile(dockerfile string, logger log.Logger) error {
	stages, margs, err := parseDockerfile(dockerfile)
	if err != nil {
		return err
	}

	// validate only 1 FROM
	if len(stages) > 1 {
		return fmt.Errorf("build.Dockerfile is not permitted to use multistage build")
	}

	// validate only permitted Commands
	for _, stage := range stages {
		for _, command := range stage.Commands {
			found := false
			for _, permittedCommand := range permittedCommandsBuild {
				if permittedCommand == strings.ToUpper(command.Name()) {
					found = true
					break
				}
			}
			if !found {
				logger.Warnf("build.Dockerfile command %s on line %d is not recommended", strings.ToUpper(command.Name()), command.Location()[0].Start.Line)
			}
		}
	}

	// validate build.Dockerfile preamble
	if len(margs) != 1 {
		return fmt.Errorf("build.Dockerfile did not start with required ARG command")
	}
	if margs[0].Args[0].Key != "base_image" {
		return fmt.Errorf("build.Dockerfile did not start with required ARG base_image command")
	}
	// sanity check to prevent panic
	if len(stages) == 0 {
		return fmt.Errorf("build.Dockerfile should have at least one stage")
	}
	if stages[0].BaseName != "${base_image}" {
		return fmt.Errorf("build.Dockerfile did not contain required FROM ${base_image} command")
	}

	return nil
}

func VerifyRunDockerfile(dockerfile string) error {
	stages, margs, err := parseDockerfile(dockerfile)
	if err != nil {
		return err
	}

	// validate only 1 FROM
	if len(stages) > 1 {
		return fmt.Errorf("run.Dockerfile is not permitted to use multistage build")
	}

	// validate FROM does not expect argument
	if len(margs) > 0 {
		return fmt.Errorf("run.Dockerfile should not expect arguments")
	}

	// sanity check to prevent panic
	if len(stages) == 0 {
		return fmt.Errorf("run.Dockerfile should have at least one stage")
	}

	// validate no instructions in stage
	if len(stages[0].Commands) != 0 {
		return fmt.Errorf("run.Dockerfile is not permitted to have instructions other than FROM")
	}

	return nil
}

func RetrieveFirstFromImageNameFromDockerfile(dockerfile string) (string, error) {
	ins, _, err := parseDockerfile(dockerfile)
	if err != nil {
		return "", err
	}
	// sanity check to prevent panic
	if len(ins) == 0 {
		return "", fmt.Errorf("expected at least one instruction")
	}
	return ins[0].BaseName, nil
}
