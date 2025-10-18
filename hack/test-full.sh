#!/usr/bin/env bash

# This script runs unit, integration and e2e tests with all optional tests
# enabled:
# - Matrix (for each runtime/language/builder c. product)
# - Podman
# - Gitlab
# - Pipelines
# - etc.
#
# (See the environment variables which allow selective overriding.)
#
# This script presumes a local testing environment set up using the
# helper scripts in ./hack and performs some precondition checks to ensure
# resources are available for the features enabled (nonexhaustive).
#     hack/binaries.sh   - Installs necessary binaries in ./hack/bin
#     hack/cluster.sh    - Start test cluster with Knative Serving/Eventing
#     hack/registry.sh   - Starts and configures a local container registry
#     hack/git-server.sh - Starts a git server in-cluster
#     hack/gitlab.sh     - Installs GitLab in-cluster

set -o errexit
set -o nounset
set -o pipefail

# Enable Optional Tests
# ---------------------
# The defaults in the e2e test implementation are a bit more conservative.
# Here we toggle on All The Things.  Note that we still allow any settings
# made explicitly in the current environment to take precidence; just setting
# new defaults which are more expansive in testing scope.
export FUNC_CLUSTER_RETRIES="${FUNC_CLUSTER_RETRIES:-5}"
export FUNC_E2E_MATRIX="${FUNC_E2E_MATRIX:-true}"
export FUNC_E2E_VERBOSE="${FUNC_E2E_VERBOSE:-true}"
export FUNC_E2E_PODMAN="${FUNC_E2E_PODMAN:-true}"
export FUNC_INT_TEKTON_ENABLED="${FUNC_INT_TEKTON_ENABLED:-1}"
export FUNC_INT_GITLAB_ENABLED="${FUNC_INT_GITLAB_ENABLED:-1}"
export FUNC_INT_GITLAB_HOSTNAME="${FUNC_INT_GITLAB_HOSTNAME:-gitlab.localtest.me}"
export FUNC_INT_PAC_HOST="${FUNC_INT_PAC_HOST:-pac-ctr.localtest.me}"


# Environment Setup
# -------------------

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Set up PATH and KUBECONFIG
export PATH="${PROJECT_ROOT}/hack/bin:${PATH}"
export KUBECONFIG="${KUBECONFIG:-${PROJECT_ROOT}/hack/bin/kubeconfig.yaml}"

# GitLab test configuration
# This is the default set by ./hack/gitlab.sh, and is overridden in CI, and
# a warning is issued that users should not only use ./hack/gitlab.sh for
# configuring test cluster available locally, such as that created by
# hack/cluster.sh
export FUNC_TEST_GITLAB_PASS="${FUNC_TEST_GITLAB_PASS:-test-password-123}"

# Generate a timestamp for use setting things which require uniqueness
TIMESTAMP=$(date +%Y%m%d%H%M%S 2>/dev/null || date +%s 2>/dev/null || echo "$(date)")

# Initialize coverage file
echo "mode: atomic" > coverage.txt

# Precondition Checks
# -------------------

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

echo ""
echo "✓ Prerequisites check passed"

# Podman Tests
# -------------
# Run Podman-specific E2E tests if enabled
if [ "${FUNC_E2E_PODMAN}" = "true" ]; then
    echo ""
    echo "Running Podman E2E tests..."
    PODMAN_START=$SECONDS
    "$(dirname "$0")/test-podman.sh"
    PODMAN_DURATION=$((SECONDS - PODMAN_START))
    PODMAN_MINS=$((PODMAN_DURATION / 60))
    PODMAN_SECS=$((PODMAN_DURATION % 60))
    echo "✓ Podman E2E tests completed successfully (duration: ${PODMAN_MINS}m ${PODMAN_SECS}s)"
fi

# Unit and Integration Tests
# --------------------------
# Run unit and integration tests together
echo ""
echo "Running unit and integration tests..."
INTEGRATION_START=$SECONDS
# go test -tags integration -timeout 60m -coverprofile=coverage-integration.txt ./... -v -run TestNewCredentialsProvider
go test -tags integration -timeout 60m -coverprofile=coverage-integration.txt ./... -v
tail -n +2 coverage-integration.txt >> coverage.txt
rm -f coverage-integration.txt 

INTEGRATION_DURATION=$((SECONDS - INTEGRATION_START))
INTEGRATION_MINS=$((INTEGRATION_DURATION / 60))
INTEGRATION_SECS=$((INTEGRATION_DURATION % 60))

echo ""
echo "✓ Unit and Integration tests completed successfully (duration: ${INTEGRATION_MINS}m ${INTEGRATION_SECS}s)"

# E2E Tests
# --------------------------

# Run E2E tests
echo ""
echo "Running E2E tests..."
E2E_START=$SECONDS
cd "${PROJECT_ROOT}/e2e"
# go test -tags e2e -timeout 120m -coverprofile=coverage-e2e.txt -coverpkg=../... -v -run TestMatrix_
go test -tags e2e -timeout 120m -coverprofile=coverage-e2e.txt -coverpkg=../... -v
tail -n +2 coverage-e2e.txt >> ../coverage.txt
rm -f coverage-e2e.txt

E2E_DURATION=$((SECONDS - E2E_START))
E2E_MINS=$((E2E_DURATION / 60))
E2E_SECS=$((E2E_DURATION % 60))

echo "✓ E2E tests completed successfully (duration: ${E2E_MINS}m ${E2E_SECS}s)"

cd "${PROJECT_ROOT}"

echo ""
echo "✓ All tests completed successfully"
