package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

// ----------------------------- META WRAPPERS ----------------------------- //

// wrapValidateError is a meta wrapper for Validate function
func wrapValidateError(err error, cmd string) error {
	if cmd != "build" && cmd != "deploy" {
		msg := fmt.Sprintf(`
Internal error during error-wrapping: specified cmd '%s' not supported`, cmd)
		return fmt.Errorf("%w\n%s", err, msg)
	}

	if errors.Is(err, fn.ErrInvalidDomain) {
		return NewErrInvalidDomain(err, cmd)
	}
	if errors.Is(err, fn.ErrInvalidNamespace) {
		return NewErrInvalidNamespace(err, cmd)
	}
	if errors.Is(err, fn.ErrConflictingImageAndRegistry) {
		return NewErrConflictImageRegistry(err, cmd)
	}
	if errors.Is(err, fn.ErrPlatformNotSupported) {
		return NewErrPlatformNotSupported(err, cmd)
	}
	return err
}

// wrapRunError wraps errors from client.Run with CLI-specific guidance
func wrapRunError(err error, address string) error {
	var portErr *fn.ErrPortUnavailableError
	if errors.As(err, &portErr) {
		if portErr.IsPermissionDenied() {
			return NewErrPortPermissionDenied(portErr.Port, address)
		}
		return NewErrPortUnavailable(err, portErr.Port)
	}
	return err
}

// wrapDeploymentError wraps errors from client.Deploy and client.RunPipeline
func wrapDeploymentError(err error) error {
	if errors.Is(err, fn.ErrInvalidKubeconfig) {
		return NewErrInvalidKubeconfig(err)
	}
	if errors.Is(err, fn.ErrClusterNotAccessible) {
		return NewErrClusterNotAccessible(err)
	}
	return err
}

// wrapPromptError wraps errors from config prompts with CLI-specific guidance
func wrapPromptError(err error, cmd string) error {
	var errNotInit *fn.ErrNotInitialized
	if errors.As(err, &errNotInit) {
		return NewErrNotInitialized(err, cmd)
	}
	if errors.Is(err, fn.ErrRegistryRequired) {
		return NewErrRegistryRequired(err, cmd)
	}
	return err
}

// ---------------------------- TYPES AND METHODS --------------------------- //

type ErrPlatformNotSupported struct {
	Err error
	Cmd string
}

func NewErrPlatformNotSupported(err error, cmd string) error {
	return &ErrPlatformNotSupported{Err: err, Cmd: cmd}
}

func (e *ErrPlatformNotSupported) Error() string {
	return fmt.Sprintf(`%v

The --platform flag is only supported with the S2I builder.

Try this:
  func %s --registry <registry> --builder=s2i --platform linux/amd64

Or remove the --platform flag:
  func %s --registry <registry>

For more options, run 'func %s --help'`, e.Err, e.Cmd, e.Cmd, e.Cmd)
}

func (e *ErrPlatformNotSupported) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //
type ErrConflictImageRegistry struct {
	Err error
	Cmd string
}

func NewErrConflictImageRegistry(err error, cmd string) error {
	return &ErrConflictImageRegistry{Err: err, Cmd: cmd}
}

func (e *ErrConflictImageRegistry) Error() string {
	return fmt.Sprintf(`%v

Cannot use both --image and --registry together. Choose one:

  Use --image for complete image name:
    func %s --image example.com/user/myfunc

  Use --registry for automatic naming:
    func %s --registry example.com/user

Note: FUNC_REGISTRY environment variable doesn't conflict with --image flag

For more options, run 'func %s --help'`, e.Err, e.Cmd, e.Cmd, e.Cmd)
}
func (e *ErrConflictImageRegistry) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //
type ErrInvalidNamespace struct {
	Err error
	Cmd string
}

func NewErrInvalidNamespace(err error, cmd string) error {
	return &ErrInvalidNamespace{Err: err, Cmd: cmd}
}

func (e *ErrInvalidNamespace) Error() string {
	return fmt.Sprintf(`%v

Invalid namespace name. Kubernetes namespaces must:
  - Contain only lowercase letters, numbers, and hyphens (-)
  - Start with a letter and end with a letter or number
  - Be 63 characters or less

Valid examples:
  func %s --namespace myapp
  func %s --namespace my-app-123

For more options, run 'func %s --help'`, e.Err, e.Cmd, e.Cmd, e.Cmd)
}

func (e *ErrInvalidNamespace) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrInvalidDomain struct {
	Err error
	Cmd string
}

func NewErrInvalidDomain(err error, cmd string) error {
	return &ErrInvalidDomain{Err: err, Cmd: cmd}
}

func (e *ErrInvalidDomain) Error() string {
	return fmt.Sprintf(`%v

Domain names must be valid DNS subdomains:
  - Lowercase letters, numbers, hyphens (-), and dots (.) only
  - Start and end with a letter or number
  - Max 253 characters total, each part between dots max 63 characters

Valid examples:
  func %s --registry ghcr.io/user --domain example.com
  func %s --registry ghcr.io/user --domain api.example.com

Note: Domain must be configured on your Knative cluster, or it will be ignored.

For more options, run 'func %s --help'`, e.Err, e.Cmd, e.Cmd, e.Cmd)
}

func (e *ErrInvalidDomain) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrInvalidKubeconfig struct {
	Err error
}

func NewErrInvalidKubeconfig(err error) error {
	return &ErrInvalidKubeconfig{Err: err}
}

func (e *ErrInvalidKubeconfig) Error() string {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config (default)"
	}
	return fmt.Sprintf(`%v

The kubeconfig file at '%s' does not exist or is not accessible.

Try this:
  export KUBECONFIG=~/.kube/config           Use default kubeconfig
  kubectl config view                        Verify current config
  ls -la ~/.kube/config                      Check if config file exists

For more options, run 'func deploy --help'`, e.Err, kubeconfigPath)
}

func (e *ErrInvalidKubeconfig) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrClusterNotAccessible struct {
	Err error
}

func NewErrClusterNotAccessible(err error) error {
	return &ErrClusterNotAccessible{Err: err}
}

func (e *ErrClusterNotAccessible) Error() string {
	errMsg := e.Err.Error()

	// Case 1: Empty/no cluster configuration in kubeconfig
	if strings.Contains(errMsg, "no configuration has been provided") ||
		strings.Contains(errMsg, "invalid configuration") {
		return fmt.Sprintf(`%v

Cannot connect to Kubernetes cluster. No valid cluster configuration found.

Try this:
  minikube start                             Start Minikube cluster
  kind create cluster                        Start Kind cluster
  kubectl cluster-info                       Verify cluster is running
  kubectl config get-contexts                List available contexts

For more options, run 'func deploy --help'`, e.Err)
	} // end if

	// Case 2: Cluster is down, network issues, auth errors, etc
	return fmt.Sprintf(`%v

Cannot connect to Kubernetes cluster.

Try this:
  kubectl cluster-info                       Verify cluster is accessible
  minikube status                            Check Minikube cluster status
  kubectl get nodes                          Test cluster connection

For more options, run 'func deploy --help'`, e.Err)
}

func (e *ErrClusterNotAccessible) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrNotInitialized struct {
	Err error
	Cmd string
}

// NewErrNotInitialized wraps an existing error (e.g., caught from a bubbled-up call)
func NewErrNotInitialized(err error, cmd string) error {
	return &ErrNotInitialized{Err: err, Cmd: cmd}
}

// NewErrNotInitializedFromPath creates the error directly when detecting uninitialized state
func NewErrNotInitializedFromPath(path, cmd string) error {
	return &ErrNotInitialized{
		Err: fmt.Errorf("'%s' does not contain an initialized function", path),
		Cmd: cmd,
	}
}

func (e *ErrNotInitialized) Error() string {
	switch e.Cmd {
	case "build":
		return fmt.Sprintf(`%v

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

For more options, run 'func build --help'`, e.Err)

	case "deploy":
		return fmt.Sprintf(`%v

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

For more options, run 'func deploy --help'`, e.Err)

	case "run":
		return fmt.Sprintf(`%v

No function found in provided path.
You need to be inside a function directory to run it (or use --path).

Try this:
  func create --language go myfunction    Create a new function
  cd myfunction                          Go into the function directory
  func run                               Run the function locally

Or if you have an existing function:
  cd path/to/your/function              Go to your function directory
  func run                              Run the function locally

For more options, run 'func run --help'`, e.Err)

	case "describe":
		return fmt.Sprintf(`%v

No function found in provided path (current directory or via --path).
The 'func describe' command shows details about a deployed function.

Try this:
  func create --language go myfunction    Create a new function
  cd myfunction                          Go into the function directory
  func deploy --registry <registry>      Deploy to cluster
  func describe                          Show deployed function details

Or if you have an existing deployed function:
  cd path/to/your/function              Go to your function directory
  func describe                         Show deployed function details

Or use --path to describe from anywhere:
  func describe --path /path/to/function

For more options, run 'func describe --help'`, e.Err)

	default:
		return e.Err.Error()
	}
}
func (e *ErrNotInitialized) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrRegistryRequired struct {
	Err error
	Cmd string
}

func NewErrRegistryRequired(err error, cmd string) error {
	return &ErrRegistryRequired{Err: err, Cmd: cmd}
}

func (e *ErrRegistryRequired) Error() string {
	switch e.Cmd {
	case "build":
		return fmt.Sprintf(`%v

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

For more options, run 'func build --help'`, e.Err)

	case "deploy":
		return fmt.Sprintf(`%v

Try this:
  func deploy --registry ghcr.io/myuser

Or set the FUNC_REGISTRY environment variable:
  export FUNC_REGISTRY=ghcr.io/myuser
  func deploy

Common registries:
  ghcr.io/myuser       GitHub Container Registry
  docker.io/myuser     Docker Hub
  quay.io/myuser       Quay.io

For more options, run 'func deploy --help'`, e.Err)

	default:
		return e.Err.Error()
	}
}

func (e *ErrRegistryRequired) Unwrap() error {
	return e.Err
}

// ----------------------------- RUN WRAPPERS ------------------------------ //

type ErrPortPermissionDenied struct {
	Err     error
	Port    string
	Address string
}

func NewErrPortPermissionDenied(port, address string) error {
	return &ErrPortPermissionDenied{Port: port, Address: address}
}

func (e *ErrPortPermissionDenied) Error() string {
	return fmt.Sprintf(`
Cannot bind to port %s: permission denied.

Port %s is a privileged port and requires administrator/root permissions.

Try this:
  sudo func run --address %s        Run with elevated permissions
  func run --address 127.0.0.1:8080          Use non-privileged port

For more options, run 'func run --help'`, e.Port, e.Port, e.Address)
}

func (e *ErrPortPermissionDenied) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrPortUnavailable struct {
	Err  error
	Port string
}

func NewErrPortUnavailable(err error, port string) error {
	return &ErrPortUnavailable{Err: err, Port: port}
}

func (e *ErrPortUnavailable) Error() string {
	return fmt.Sprintf(`%v

Port %s is not available.

The port may be in use by another process, or you may not have permission to bind to it.

Try this:
  func run --address 127.0.0.1:8080          Use a different port
  lsof -i :%s                                Check if port is in use (Linux/Mac)

For more options, run 'func run --help'`, e.Err, e.Port, e.Port)
}

func (e *ErrPortUnavailable) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrInvalidAddress struct {
	Err     error
	Address string
}

func NewErrInvalidAddress(err error, address string) error {
	return &ErrInvalidAddress{Err: err, Address: address}
}

func (e *ErrInvalidAddress) Error() string {
	return fmt.Sprintf(`
Invalid address format '%s': address must include both host and port
Address format: host:port

Examples:
  127.0.0.1:8080    Localhost only
  0.0.0.0:8080      All interfaces (IPv4)
  [::]:8080         All interfaces (IPv6)

For more options, run 'func run --help'`, e.Address)
}

func (e *ErrInvalidAddress) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrInvalidPort struct {
	Err  error
	Port string
}

func NewErrInvalidPort(err error, port string) error {
	return &ErrInvalidPort{Err: err, Port: port}
}

func (e *ErrInvalidPort) Error() string {
	return fmt.Sprintf(`
Invalid port '%s': port must be a number between 1 and 65535

Examples:
  func run --address 127.0.0.1:8080
  func run --address 0.0.0.0:9090

For more options, run 'func run --help'`, e.Port)
}

func (e *ErrInvalidPort) Unwrap() error {
	return e.Err
}

// -------------------------------------------------------------------------- //

type ErrListClusterConnection struct {
	Err error
}

func NewErrListClusterConnection(err error) error {
	return &ErrListClusterConnection{Err: err}
}

func (e *ErrListClusterConnection) Error() string {
	return fmt.Sprintf(`%v

Cannot connect to Knative cluster

The 'func list' command shows functions deployed to your Knative cluster.

To use this command, you need:
  1. A running Kubernetes cluster
  2. Knative Serving installed on the cluster
  3. kubectl configured to access your cluster

Workflow:
  func create --language go myfunction    Create a function
  func deploy --registry <registry>       Deploy to cluster
  func list                               See your deployed functions

Troubleshooting:
  kubectl get pods -n knative-serving     Check Knative installation
  kubectl config current-context          Verify cluster connection

Installation guide: https://knative.dev/docs/serving/#installation`, e.Err)
}

func (e *ErrListClusterConnection) Unwrap() error {
	return e.Err
}
