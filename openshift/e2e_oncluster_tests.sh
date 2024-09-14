#!/usr/bin/env bash
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# Runs basic lifecycle E2E tests against kn func cli for a given language/runtime.
# By default it will run e2e tests against 'func' binary, but you can change it to use 'kn func' instead
#
# The following environment variable can be set in order to customize e2e execution:
#
# E2E_USE_KN_FUNC    When set to "true" indicates e2e to issue func command using kn cli.
#
# E2E_REGISTRY_URL   Indicates a specific registry (i.e: "quay.io/user") should be used. Make sure
#                    to authenticate to the registry (i.e: docker login ...) prior to execute the script
#                    By default it uses "ttl.sh" registry
#
# E2E_FUNC_BIN_PATH  Path to func binary. Derived by this script when not set
#
# E2E_RUNTIMES       List of runtimes (space separated) to execute TestRuntime.
#

set -o errexit
set -o nounset
set -o pipefail

FUNC_UTILS_IMG="${FUNC_UTILS_IMG:-ghcr.io/knative/func-utils:latest}"
LDFLAGS="-X knative.dev/func/pkg/k8s.SocatImage=${FUNC_UTILS_IMG}"
LDFLAGS+=" -X knative.dev/func/pkg/k8s.TarImage=${FUNC_UTILS_IMG}"
LDFLAGS+=" -X knative.dev/func/pkg/pipelines/tekton.DeployerImage=${FUNC_UTILS_IMG}"
export GOFLAGS="'-ldflags=${LDFLAGS}'"

source "$(go run knative.dev/hack/cmd/script e2e-tests.sh)"

pushd "$(dirname "$0")/.."

export BUILD_NUMBER=${BUILD_NUMBER:-$(head -c 128 < /dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | head -c 8)}
export ARTIFACT_DIR="${ARTIFACT_DIR:-$(dirname "$(mktemp -d -u)")/build-${BUILD_NUMBER}}"
export ARTIFACTS="${ARTIFACTS:-${ARTIFACT_DIR}}/kn-func/e2e-oncluster-tests"
mkdir -p "${ARTIFACTS}"

export E2E_REGISTRY_URL="${E2E_REGISTRY_URL:-ttl.sh/knfuncci$(head -c 128 </dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | head -c 6)}"
export E2E_FUNC_BIN_PATH="${E2E_FUNC_BIN_PATH:-$(pwd)/func}"
export E2E_USE_KN_FUNC="false"
export E2E_GIT_SERVER_PODNAME="gitserver"
export E2E_GIT_SERVER_ROUTE_URL="http://$(oc get route gitserver -o jsonpath='{.spec.host}')"
FUNC_REPO_REF="${FUNC_REPO_REF:-openshift-knative/kn-plugin-func}"
FUNC_REPO_BRANCH_REF="${FUNC_REPO_BRANCH_REF:-release-next}"

# Ensure 'func' binary is built
if [[ ! -f "$E2E_FUNC_BIN_PATH" ]]; then
  echo "=== building func binary"
  env FUNC_REPO_REF=${FUNC_REPO_REF} FUNC_REPO_BRANCH_REF=${FUNC_REPO_BRANCH_REF} make build
fi

# For now, let's skips tests that depends on Podman/Docker on Openshift CI
if [[ "${OPENSHIFT_CI}" == "true" ]] ; then
  mv ./test/oncluster/scenario_from-cli-local_test.go ./test/oncluster/scenario_from-cli-local_test.skip
fi

# Execute on cluster tests (s2i only)
echo "=== running e2e oncluster test"
export FUNC_BUILDER="s2i"
export FUNC_INSECURE="true"
go_test_e2e -v -timeout 90m -tags="oncluster" ./test/oncluster/ || fail_test 'kn-func e2e tests'
ret=$?

popd
exit $ret
