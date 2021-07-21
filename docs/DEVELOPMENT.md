# Development

This document details how to get started contributing to the project.  This includes building and testing the core, templates and how to update them, and usage of the optional integration testing tooling.

## Building

To build the core project, run `make` from the repository root.  This will result in a `func` binary being generated.  Run `make help` for additional targets and `./func help` for usage-centric documentation.

Building currently requires that `pkger` be available in your `$PATH`.  This can be installed with `go get github.com/markbates/pkger/cmd/pkger`.  See [Templates](#templates) below for more information on this dependency.

To remove built artifacts, use `make clean`.

## Testing

To run core unit tests, use `make test`.

## Linting

Before submitting code in a Pull Request, please run `make check` and resolve any errors.  This creates and runs `bin/golangci-lint`.  For settings such as configured linters, see [.golangci.yaml](../.golangci.yaml).


## Templates

When a new Function is created, a few files are placed in the new Function's directory.  This includes source code illustrating a minimal Function of the requested type (language runtime and function signature) as well as Function metadata (`func.yaml`).

The source of these templates is `./templates`; a directory subdivided by language and template name.  For example, the Go HTTP template is located in `./templates/go/http`.  The client library and CLI are self-contained by encoding this directory as `pkged.go`.   Therefore any updates to templates requires re-generating this file.

When changes are made to a template's source code, regenerate `pkged.go` by running `make pkged.go`.  It is also important to run the unit tests of the template modified.  For example, to run the unit tests of the Go templates, use `make test-go`.  For a list of available make targets, use `make help`.


## Integration Testing

*Integration tests are run automatically* for all pull requests from within the GitHub Pull Request.  This process includes creating and configuring a lightweight cluster.

If you would like to run integration tests prior to opening a pull request against origin, you can enable Actions in your fork of this repository and create a pull request to your own main branch.

If you would like to run integraiton tests locally, or would like to use the CLI / Client Library directly against a local cluster, the cluster allocation script can be used locally as well:


###  Prerequisites

The cluster allocation script requires [jq](https://stedolan.github.io/jq/), [yq](https://github.com/kislyuk/yq), [kubectl](https://kubernetes.io/docs/tasks/tools/), [kind](https://kind.sigs.k8s.io/docs/user/quick-start/), docker and python.

Please note that the version of `yq` required is installed via `pip3 install yq` or `brew install python-yq`


### Allocate

Allocate a new local cluster by running `hack/allocate.sh`.


### Registry

The allocation script sets up a local container registry and connects it to the cluster.  This registry must be set as trusted and its address entered in the local hosts file.  This is a one-time configuration and on Linux can be accomplished by running `hack/registry.sh`. 

On other systems, add `127.0.0.1 kind-registry` to your local hosts file and `"insecure-registries" = ["kind-registry:5000"]` to your docker daemon config (`docker/daemon.json`).


### Using the Cluster

Once the cluster has been allocated, the `func` CLI (or client library) will automatically use it (see the [Kubeconfig Docs](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) for more)

Functions will be available at the address `[Function Name].default.127.0.0.1.sslip.io`

To run integration tests, use `make test-integration`.


### Cleanup

The cluster and registry can be deleted by running `hack/delete.sh`







