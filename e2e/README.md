# E2E (end-to-end) Tests

E2E test confirm the functionality of the system end-to-end from the
perspective of a user employing the Functions CLI `func`, either standalone
or as a plugin to `kn` (`kn func`).

E2E tests are designed in a way that they be easily runnable (and thus
debuggable) locally by a developer, in addition to remotely in CI as
acceptance criteria for pull requests.

## Runnning E2Es locally: a Quick-start

- `./hack/install-binaries.sh`  Fetch binaries into `./hack/bin`
- `./hack/registry.sh`          Configure system for insecure local registrires
- `./hack/allocate.sh`          Create a cluster and kube config in `./hack/bin`
- `make test-e2e`               Run all tests using these bins and cluster
- `./hack/delete.sh`            Remove the cluster


## Overview

Tests themselves are separated into five categories:  Core, Metadata,
Repository, Remote, and Matrix.

Core tests include checking the basic CRUDL operations; Create, Read, Update,
Delete and List.  Creation is implemented as `func init`, running the function
locally with `func run`, and running the cluster with `func deploy`. Reading is
implemented as `func describe`.  Updating, which ensures that an updated
function replaces the old, is implemented as `func deploy`.  Finally,
`func list` implements a standard listing operation.

Metadata tests ensure that manipulation of a Function's metadata is correctly
carried to the final Function.  Metadata includes environment variables,
labels, volumes, secrets and event subscriptions.

Repository tests confirm features which involve working with git repositories.
This includes operations such as building locally from source code located in
a remote repository, building a specific revision, etc.

Remote tests confirm features related to building and deploying remotely
via in-cluster builds, etc.

Matrix tests is a larger set which checks operations which differ in
implementation between language runtimes.  The primary operations which
differ and must be checked for each runtime are creation and running locally.
Therefore, the runtime tests execute for each language, for each template, for
each builder.  As a side-effect of the test implementation, "func invoke" is
also tested.

## Prerequisites

These tests expect a compiled binary, which will be executed utilizing a
cluster configured to run Functions, as well as an available and authenticated
container registry.  These can be configured locally for testing by using
scripts in `../hack`:

- `install-binaries.sh`: Installs executables needed for cluster setup and
  configuration into hack/bin.

- `regsitry.sh`: Configures the local Podman or Docker to allow unencrypted
  communication with local registries.

- `allocate.sh`: Creates a local Function-ready cluster.

- `delete.sh`: Removes the cluster and registry.  Using this to recreate the
  cluster between test runs will ensure the cluster is in a clean initial state.

## Options

The suite accepts environment variables which alter the default behavior:

`FUNC_E2E_BIN`: sets the path to the binary to use for the E2E tests.  This is
by default the binary created when `make` is run in the repository root.
Note that if providing a relative path, this path is relative to this test
package, not the directory from which `go test` was run.

`FUNC_E2E_PLUGIN`: if set, the command run by the tests will be
`${FUNC_E2E_BIN} func`, allowing for running all tests when func is installed
as a plugin; such as when used as a plugin for the Knative cluster admin
tool `kn`.  The value should be set to the name of the subcommand for the
func plugin (usually `func`).  For example to run E2E tests on `kn` with
the `kn-func` plugged in use `FUNC_E2E_BIN=/path/to/kn FUNC_E2E_PLUGIN=func`.

`FUNC_E2E_REGISTRY`: if provided, tests will use this registry (in form
`registry.example.com/user`) instead of the test suite default of
`localhost:50000/func`.

`FUNC_E2E_MATRIX_RUNTIMES`: Sets which runtimes will be tested during the matrix
tests. By default matrix test are not enabled unless this value is passed.
Note that the core tests always use the `go` runtime.

`FUNC_E2E_MATRIX_BUILDERS`: Sets which builders will be tested during the matrix
tests.  By default matrix tests are not enabled unless this value is passed.
Note that core tests always use the `host` builder.

`FUNC_E2E_KUBECONFIG`: The path to the kubeconfig to be used by tests.  This
defaults to `../hack/bin/kubeconfig.yaml`, which is created when using the
`../hack/allocate.sh` script to set up a test cluster.

`FUNC_E2E_GOCOVERDIR`: The path to use for Go coverage data reported by these
tests.  This defaults to `../.coverage`.

`FUNC_E2E_GO`: the path to the `go` binary tests should use when running
outside of a container (host builder, or runner with `--container=false`).  This
can be used to test against specific go versions.  Defaults to the go binary
found in the current session's PATH.

`FUNC_E2E_GIT`: the path to the `git` binary tests should provide to the commands
being tested for git-related activities.   Defaults to the git binary
found in the current session's PATH.

`FUNC_E2E_VERBOSE`: instructs the test suite to run all commands in
verbose mode.

## Running

From the root of the repository, run `make test-e2e`.  This will compile
the current source, creating the binary `./func` if it does not already exist,
or is out of date. It will then run `go test -tags e2e ./e2e`.  By default the
tests will use the locally compiled `func` binary unless `FUNC_E2E_BIN` is
provided.

The test cache is cleaned before running the tests when using the `make`
targets to eliminate situations where changes to the environment or system
can result in invalid results due to caching.  Caching can be utilized by
running tests directly (eg `go test -tags e2e ./e2e`).

Tests follow a naming convention to allow for manually testing subsets.  For
example, To run only "core" tests, run `make` to update the binary to test,
then `go test -tags e2e -run TestCore ./e2e`. Subsets include:
- TestCore
- TestMetadata
- TestRepository
- TestMatrix

## Cleanup

The tests do attempt to clean up after themselves, but since a test failure is
definitionally the entering of an unknown state, it is suggested to delete
the cluster between full test runs. To remove the local cluster, use the
`delete.sh` script described above.

## TODO
- Core tests should also test w/ CloudEvents template
- Ensure matrix tests accept E2E_RUNTIMES w/ empty indicating skip.
- See if presubmit tests should be included
- Create .coverage directory
- Note that oncluster tests used ttl.sh for their registry:
    REGISTRY_PROJ=knfunc$(head -c 128 </dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | fold -w 8 | head -n 1)
  This should be replaced with something we control, for example
  docker.io/functions-dev
- If k8s.IsOpenShift() registry= k8s.GetDefaultOpenShiftRegistry()
  (see test/common/config.go)
- Ensure output is stripped of the clock in the slow matrix tests:
  (see test/common/outcleaner.go).  Most likely by enabling verbose or by
  removing the clock when !interactiveTerminal.
- Measure
  - Linecount reduction
  - Filecount reduction
  - Package reduction
  - Tags reduction
  - Scripts reduction
  - Runtime reduction
  - Completeness increase
- check if patchOrCreateDocerConfigFile in test/e2e/main_test.go is needed.

## Migration Notes

Replace script executions with "make test-e2e" (backwards compatibility for
environment variables is implemented).

Coverage data is formatted (go tool covdata textfmt) as a part of the GitHub
action.

Coverage data is now all held under the single tag "e2e".

The binary is compiled automatically via the make target.


## Changelog
  - Now supports testing func when a plugin of a different name.
  - Now supports running specific runtimes rather than the prior version which
    supported one or all.
  - Uses sensible defaults for environment variables to reduce setup when
    running locally.
  - Removes redundant `go test` flags.
  - Now supports specifying builders.
  - Subsets of test can be specified using name prefixes --run=TestCore etc.
  - Cluster includes Tekton and supporting tasks by default.
  - Combines all "scenarios" into a single suite.
  - Removes all interstitial shell scripts in favor of idomatic go tags.
  - removed test-slowing `go clean -testcache` as tests are now structured
    in a way that should.
  - Removes dependency on the free/flaky ttl.sh service.
  - Always connects command output to stdout/stderr.
  - Uses Go and the Host builder for all tests by default.
  - Removes all testing abstraction layers (test helper functions only).
  - Removes any "common" or "util" packages.
  - Combines most tests into a single "e2e" suite
  - Combines most GitHub Actions into a single "test" workflow
  - Combines most cluster setup into allocate.sh
  - Removes catchall "common" and "util" packages and files 
  - Combines tests into single e2e_test.go
  - Utilizes the functions client library in place of k8s/knative library
    where applicable
  - Removes all interactive tests (func UX complete overhaul is in progress)


