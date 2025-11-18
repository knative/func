# E2E (end-to-end) Tests

E2E tests confirm the functionality of the system end-to-end from the
perspective of a user employing the Functions CLI `func`, either standalone
or as a plugin to `kn` (`kn func`).

E2E tests are designed in a way that they can be easily runnable (and thus
debuggable) locally by a developer, in addition to remotely in CI as
acceptance criteria for pull requests.

## Running E2Es locally: a Quick-start

- `./hack/binaries.sh`          Fetch binaries into `./hack/bin`
- `./hack/registry.sh`          (once) Configure insecure local registry
- `./hack/cluster.sh`           Create a cluster and kube config in `./hack/bin`
- `make test-full`               Run all tests using these binaries and cluster
- `./hack/delete.sh`            Remove the cluster


## Overview

Tests themselves are separated into categories:  Core, Metadata,
Remote, Podman, and Matrix.

Core tests include checking the basic CRUDL operations; Create, Read, Update,
Delete and List.  Creation is implemented as `func init`, running the function
locally with `func run`, and running the cluster with `func deploy`. Reading is
implemented as `func describe`.  Updating, which ensures that an updated
function replaces the old, is implemented as `func deploy`.  Finally,
`func list` implements a standard listing operation.  Tests also confirm
the core features function when referring to a remote repository, using
templates (repositories) and other core variants.

Metadata tests ensure that manipulation of a Function's metadata is correctly
carried to the final Function.  Metadata includes environment variables,
labels, volumes, secrets and event subscriptions.

Remote tests confirm features related to building and deploying remotely
via in-cluster builds, etc.

Podman tests ensure that the Podman container engine is also supported.  Note
that these tests require that 'podman' and 'ssh' are available in your path.

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

- `binaries.sh`: Installs executables needed for cluster setup and
  configuration into hack/bin.

- `registry.sh`: Configures the local Podman or Docker to allow unencrypted
  communication with local registries.

- `cluster.sh`: Creates a local Function-ready cluster with Knative, Tekton,
  and GitLab support. Includes DNS configuration for localtest.me domains.

- `delete.sh`: Removes the cluster and registry.  Using this to recreate the
  cluster between test runs will ensure the cluster is in a clean state.

- `gitlab.sh`: Sets up GitLab instance for testing Git-based deployments.  Only
required if gitlab tests are enabled.

## Options

The suite accepts environment variables which alter the default behavior:

`FUNC_E2E_BIN`: sets the path to the binary to use for the E2E tests.  This is
by default the binary created when `make` is run in the repository root.
Note that if providing a relative path, this path is relative to this test
package, not the directory from which `go test` was run.

`FUNC_E2E_PLUGIN`: if set, the command run by the tests will be
`${FUNC_E2E_BIN} ${FUNC_E2E_PLUGIN}`, allowing for running all tests when func is installed
as a plugin; such as when used as a plugin for the Knative cluster admin
tool `kn`.  The value should be set to the name of the subcommand for the
func plugin (usually `func`).  For example to run E2E tests on `kn` with
the `kn-func` plugged in use `FUNC_E2E_BIN=/path/to/kn FUNC_E2E_PLUGIN=func`.

`FUNC_E2E_REGISTRY`: if provided, tests will use this registry (in form
`registry.example.com/user`) instead of the test suite default of
`localhost:50000/func`. This is used for local builds that push from the
developer's machine to the registry.

`FUNC_E2E_CLUSTER_REGISTRY`: specifies the cluster-internal registry URL used
for in-cluster (remote) builds with Tekton. This registry must be accessible
from within the cluster. Format: `registry.namespace.svc.cluster.local:port/path`.
Defaults to `registry.default.svc.cluster.local:5000/func`.

`FUNC_E2E_MATRIX_RUNTIMES`: Sets which runtimes will be tested during the matrix
tests. By default matrix test are not enabled unless this value is passed.
Note that the core tests always use the `go` runtime.

`FUNC_E2E_MATRIX_BUILDERS`: Sets which builders will be tested during the matrix
tests.  By default matrix tests are not enabled unless this value is passed.
Note that core tests always use the `host` builder.

`FUNC_E2E_KUBECONFIG`: The path to the kubeconfig to be used by tests.  This
defaults to `../hack/bin/kubeconfig.yaml`, which is created when using the
`../hack/cluster.sh` script to set up a test cluster.

`FUNC_E2E_GOCOVERDIR`: The path to use for Go coverage data reported by these
tests.  This defaults to `../.coverage`.

`FUNC_E2E_GO`: the path to the `go` binary tests should use when running
outside of a container (host builder).  This
can be used to test against specific go versions.  Defaults to the go binary
found in the current session's PATH.

`FUNC_E2E_GIT`: the path to the `git` binary tests should provide to the commands
being tested for git-related activities.   Defaults to the git binary
found in the current session's PATH.

`FUNC_E2E_VERBOSE`: instructs the test suite to run all commands in
verbose mode. When set to "true", the `-v` flag is automatically added to all
func commands executed during tests. Defaults to "false".

`FUNC_E2E_CLEAN`: controls whether tests clean up deployed functions after
completion. When set to "true" (default), functions are deleted after each
test. Set to "false" to leave functions deployed for debugging. This speeds
up test execution when the same cluster is reused across multiple test runs.

`FUNC_E2E_DOCKER_HOST`: sets the DOCKER_HOST environment variable for
container operations during tests. This is useful when using a remote Docker
daemon or when the Docker socket is not at the default location.

`FUNC_E2E_HOME`: sets a custom home directory path for test execution. By
default, tests create a temporary `.func_e2e_home` directory within each
test's clean environment. Use this to debug tests with a persistent home
directory, but note that all tests in the invocation will share this home.

`FUNC_E2E_KUBECTL`: specifies the path to the kubectl binary used during
tests. Defaults to the kubectl found in PATH. Tests use kubectl to manipulate
cluster state as necessary, such as creating secrets and configmaps.

`FUNC_E2E_MATRIX`: enables comprehensive matrix testing across different
combinations of runtimes, builders, and templates. When set to "true",
the test suite will run tests for all supported permutations. Defaults to
"false" for faster test execution.

`FUNC_E2E_MATRIX_TEMPLATES`: sets which templates will be tested during matrix
tests. Accepts a comma-separated list (e.g., "http,cloudevents"). By default,
both "http" and "cloudevents" templates are tested when matrix tests are enabled.

`FUNC_E2E_DOMAIN`: specifies the DNS domain suffix used for function URLs
during tests. Defaults to "localtest.me". When using a custom domain, ensure
it is configured in both the cluster's CoreDNS setup and Knative serving
config-domain ConfigMap. The URL pattern is `http://{function}.{namespace}.{domain}`.
For local development, "localtest.me" is recommended as it automatically resolves
to 127.0.0.1 without additional DNS configuration.

`FUNC_E2E_NAMESPACE`: specifies the Kubernetes namespace where functions will be
deployed during tests. Defaults to "default". When using a custom namespace,
ensure DNS is configured for `{function}.{namespace}.{domain}` patterns.
This requires corresponding DNS or ingress configuration in your cluster.

`FUNC_E2E_PODMAN`: enables tests specifically for the Podman container engine.
When set to "true", tests will verify that functions can be built and deployed
using Podman with both Pack and S2I builders. Requires `FUNC_E2E_PODMAN_HOST`
to be set.

`FUNC_E2E_PODMAN_HOST`: specifies the Podman socket path for Podman-specific
tests. This is required when `FUNC_E2E_PODMAN` is enabled. Example:
"unix:///run/user/1000/podman/podman.sock" for rootless Podman.

`FUNC_CLUSTER_RETRIES`: controls the number of retry attempts for cluster
allocation. Defaults to "1" (no retries). Set to a higher value (e.g., "5")
for environments with flaky cluster setup. Used by the cluster.sh script.

`FUNC_INT_TEKTON_ENABLED`: enables Tekton-specific tests. Set to "1" to include
tests that verify Tekton pipeline functionality. Defaults to disabled.

`FUNC_INT_GITLAB_ENABLED`: enables GitLab-specific tests. Set to "1" to include
tests that verify GitLab integration. Requires gitlab.sh to be run. Defaults
to disabled.

`FUNC_E2E_TOOLS`: specifies the path to supporting tools. Defaults to
"../hack/bin" relative to the e2e directory.

`FUNC_E2E_TESTDATA`: specifies the path to supporting testdata. Defaults to
"./testdata" relative to the e2e directory.

## Running

From the root of the repository, run `make test-full`.  This will compile
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
- TestRemote
- TestMatrix

## Cleanup

The tests do attempt to clean up after themselves, but since a test failure is
definitionally the entering of an unknown state, it is suggested to delete
the cluster between full test runs. To remove the local cluster, use the
`delete.sh` script described above.  If you do plan to remove the cluster, tests
can be sped up by disabling cleanup with FUNC_E2E_CLEAN=false

## Migration Notes

Replace script executions with "make test-full" (backwards compatibility for
environment variables is implemented).

The binary is compiled automatically via the make target.

