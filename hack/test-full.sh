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
# Run full test suite with all features enabled
# This script sets up the environment for comprehensive testing including
# Podman, Tekton, GitLab, and matrix tests
#

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Generate a timestamp for use setting things which require uniqueness
TIMESTAMP=$(date +%Y%m%d%H%M%S 2>/dev/null || date +%s 2>/dev/null || echo "$(date)")

# Set up PATH and KUBECONFIG
export PATH="${PROJECT_ROOT}/hack/bin:${PATH}"
export KUBECONFIG="${KUBECONFIG:-${PROJECT_ROOT}/hack/bin/kubeconfig.yaml}"

# Set up test environment variables
# The defaults in the e2e test implementation are a bit more conservative.
# Here we toggle on All The Things.  Note that we still allow any settings
# made explicitly in the current environment to take precidence; just setting
# new defaults which are more expansive in testing scope.
export FUNC_ALLOCATE_RETRIES="${FUNC_ALLOCATE_RETRIES:-5}"
export FUNC_E2E_MATRIX="${FUNC_E2E_MATRIX:-true}"
export FUNC_E2E_VERBOSE="${FUNC_E2E_VERBOSE:-true}"
export FUNC_E2E_PODMAN="${FUNC_E2E_PODMAN:-true}"
export TEKTON_TESTS_ENABLED="${TEKTON_TESTS_ENABLED:-1}"
export GITLAB_TESTS_ENABLED="${GITLAB_TESTS_ENABLED:-0}"  # FIXME: Default to 1
export GITLAB_HOSTNAME="${GITLAB_HOSTNAME:-gitlab.localtest.me}"
export PAC_CONTROLLER_HOSTNAME="${PAC_CONTROLLER_HOSTNAME:-pac-ctr.localtest.me}"

# GitLab test configuration
# This is the default set by ./hack/gitlab.sh, and is overridden in CI, and
# a warning is issued that users should not only use ./hack/gitlab.sh for
# configuring test cluster available locally, such as that created by
# hack/cluster.sh
export GITLAB_ROOT_PASSWORD="${GITLAB_ROOT_PASSWORD:-test-password-123}"

# Check if binaries are installed
if [ ! -d "${PROJECT_ROOT}/hack/bin" ]; then
    echo "ERROR: hack/bin directory not found!"
    echo "Please run ./hack/install-binaries.sh first to install required tools."
    exit 1
fi
MISSING_BINS=""
for bin in kubectl kind; do
    # Check with and without .exe for Windows compatibility
    if [ ! -f "${PROJECT_ROOT}/hack/bin/${bin}" ] && [ ! -f "${PROJECT_ROOT}/hack/bin/${bin}.exe" ]; then
        MISSING_BINS="${MISSING_BINS} ${bin}"
    fi
done

if [ -n "${MISSING_BINS}" ]; then
    echo "ERROR: Required binaries not found:${MISSING_BINS}"
    echo "Please run ./hack/binaries.sh to install required tools."
    exit 1
fi

# Check if cluster is allocated
if [ ! -f "${KUBECONFIG}" ]; then
    echo "ERROR: KUBECONFIG not found at ${KUBECONFIG}"
    echo "Please run ./hack/allocate.sh to set up a test cluster."
    exit 1
fi

# Verify cluster connectivity
if ! kubectl cluster-info &>/dev/null; then
    echo "ERROR: Cannot connect to Kubernetes cluster"
    echo "KUBECONFIG: ${KUBECONFIG}"
    echo "Please ensure your cluster is running and KUBECONFIG is valid."
    echo "You may need to run ./hack/allocate.sh"
    kubectl cluster-info
    kin
    exit 1
fi

# Check if GitLab is installed (if GitLab tests are enabled)
if [ "${GITLAB_TESTS_ENABLED}" = "1" ]; then
    if ! kubectl get namespace gitlab &>/dev/null; then
        echo "ERROR: GitLab namespace not found"
        echo "Please run ./hack/gitlab.sh to install GitLab"
        exit 1
    fi
fi

# Check if Podman is available (if Podman tests are enabled)
if [ "${FUNC_E2E_PODMAN}" = "true" ]; then
    if ! command -v podman &>/dev/null; then
        echo "ERROR: Podman not found"
        echo "Please install Podman to run Podman tests"
        exit 1
    fi
fi

echo "✓ Prerequisites check passed"
echo ""

# Check Podman service if Podman tests are enabled
if [ "${FUNC_E2E_PODMAN}" = "true" ]; then
    # Check if Podman service is running
    if ! podman info &>/dev/null; then
        echo "ERROR: Podman service is not running"
        echo "Please start Podman service with: podman system service --time=0 &"
        exit 1
    fi
    
    # Get the socket path
    # On macOS/Windows, we need the host-accessible socket from podman machine
    # On Linux, we can use the socket from podman info
    if [[ "$OSTYPE" == "darwin"* ]] || [[ "$OSTYPE" == "msys" ]]; then
        # Try to get the socket from podman machine inspect
        MACHINE_NAME="${PODMAN_MACHINE:-podman-machine-default}"
        PODMAN_SOCKET="$(podman machine inspect "${MACHINE_NAME}" --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null || true)"
        
        if [ -z "${PODMAN_SOCKET}" ]; then
            # Fallback: check if DOCKER_HOST is already set by podman
            if [ -n "${DOCKER_HOST}" ]; then
                export FUNC_E2E_PODMAN_HOST="${DOCKER_HOST}"
            else
                echo "Warning: Could not determine Podman socket path"
                echo "Try running: podman machine stop && podman machine start"
                echo "Then use the DOCKER_HOST value it provides"
                PODMAN_SOCKET="$(podman info -f '{{.Host.RemoteSocket.Path}}')"
                export FUNC_E2E_PODMAN_HOST="unix://${PODMAN_SOCKET}"
            fi
        else
            export FUNC_E2E_PODMAN_HOST="unix://${PODMAN_SOCKET}"
        fi
    else
        # Linux: use the socket from podman info
        PODMAN_SOCKET="$(podman info -f '{{.Host.RemoteSocket.Path}}')"
        if [[ "${PODMAN_SOCKET}" == unix://* ]]; then
            export FUNC_E2E_PODMAN_HOST="${PODMAN_SOCKET}"
        else
            export FUNC_E2E_PODMAN_HOST="unix://${PODMAN_SOCKET}"
        fi
    fi
    echo "Podman socket: ${FUNC_E2E_PODMAN_HOST}"
fi

# Initialize coverage file
echo "mode: atomic" > coverage.txt

# Run unit and integration tests together
# echo ""
# echo "Running unit and integration tests..."
# go test -tags integration -timeout 60m -coverprofile=coverage-integration.txt ./... -v
# tail -n +2 coverage-integration.txt >> coverage.txt
# rm -f coverage-integration.t -run TestMetadata_Labels_Removext

# Run E2E tests
echo ""
echo "Running E2E tests..."
cd "${PROJECT_ROOT}/e2e"
# go test -tags e2e -timeout 60m -coverprofile=coverage-e2e.txt -coverpkg=../... -v -run TestMatrix_Remote
go test -tags e2e -timeout 60m -coverprofile=coverage-e2e.txt -coverpkg=../... -v
tail -n +2 coverage-e2e.txt >> ../coverage.txt
rm -f coverage-e2e.txt

cd "${PROJECT_ROOT}"
echo ""
echo "Coverage report created: coverage.txt"

echo ""
echo "All tests completed successfully!"
