package cmd

import (
	"fmt"
)

// wrapNotInitializedError wraps an ErrNotInitialized error with CLI-specific guidance
func wrapNotInitializedError(err error, command string) error {
	switch command {
	case "build":
		return fmt.Errorf(`%w

No function found in provided path (current directory or via --path).
You need to be in a function directory (or use --path).

Try this:
  func create --language go myfunction    Create a new function
  cd myfunction                          Go into the function directory
  func build --registry <registry>       Build the function container

Or navigate to an existing function:
  cd path/to/your/function
  func build --registry <registry>

Or use --path flag:
  func build --path /path/to/function --registry <registry>

For more options, run 'func build --help'`, err)

	case "deploy":
		return fmt.Errorf(`%w

No function found in provided path (current directory or via --path).
You need to be in a function directory (or use --path).

Try this:
  func create --language go myfunction    Create a new function
  cd myfunction                          Go into the function directory
  func deploy --registry <registry>      Deploy to the cloud

Or navigate to an existing function:
  cd path/to/your/function
  func deploy --registry <registry>

Or use --path to deploy from anywhere:
  func deploy --path /path/to/function --registry <registry>

For more options, run 'func deploy --help'`, err)

	default:
		return err
	}
}

// wrapRegistryRequiredError wraps an ErrRegistryRequired error with CLI-specific guidance
func wrapRegistryRequiredError(err error, command string) error {
	switch command {
	case "build":
		return fmt.Errorf(`%w

Try this:
  func build --registry ghcr.io/myuser    Build with registry

Or set the FUNC_REGISTRY environment variable:
  export FUNC_REGISTRY=ghcr.io/myuser
  func build

Common registries:
  ghcr.io/myuser       GitHub Container Registry
  docker.io/myuser     Docker Hub
  quay.io/myuser       Quay.io

Or specify full image name:
  func build --image ghcr.io/myuser/myfunction:latest

For more options, run 'func build --help'`, err)

	case "deploy":
		return fmt.Errorf(`%w

Try this:
  func deploy --registry ghcr.io/myuser

Or set the FUNC_REGISTRY environment variable:
  export FUNC_REGISTRY=ghcr.io/myuser
  func deploy

Common registries:
  ghcr.io/myuser       GitHub Container Registry
  docker.io/myuser     Docker Hub
  quay.io/myuser       Quay.io

For more options, run 'func deploy --help'`, err)

	default:
		return err
	}
}
