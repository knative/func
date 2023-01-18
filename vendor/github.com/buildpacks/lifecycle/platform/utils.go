package platform

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/log"
)

func appendNotEmpty(slice []string, elems ...string) []string {
	for _, v := range elems {
		if v != "" {
			slice = append(slice, v)
		}
	}
	return slice
}

func ensureSameRegistry(firstRef string, secondRef string) error {
	if firstRef == secondRef {
		return nil
	}
	firstRegistry, err := parseRegistry(firstRef)
	if err != nil {
		return err
	}
	secondRegistry, err := parseRegistry(secondRef)
	if err != nil {
		return err
	}
	if firstRegistry != secondRegistry {
		return fmt.Errorf("writing to multiple registries is unsupported: %s, %s", firstRegistry, secondRegistry)
	}
	return nil
}

func parseRegistry(providedRef string) (string, error) {
	ref, err := name.ParseReference(providedRef, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func readStack(stackPath string, logger log.Logger) (StackMetadata, error) {
	var stackMD StackMetadata
	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no stack metadata found at path '%s'\n", stackPath)
		} else {
			return StackMetadata{}, err
		}
	}
	return stackMD, nil
}
