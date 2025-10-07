#!/usr/bin/env bash

# Runs Podman E2E tests

set -o errexit
set -o nounset
set -o pipefail

# Enable the Podman Tests
export FUNC_E2E_PODMAN="true"

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Set up PATH and KUBECONFIG
export PATH="${PROJECT_ROOT}/hack/bin:${PATH}"
export KUBECONFIG="${KUBECONFIG:-${PROJECT_ROOT}/hack/bin/kubeconfig.yaml}"


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

echo ""
echo "✓ Prerequisites check passed"

# Podman Setup
# -------------
# Sets FUNC_E2E_PODMAN_HOST to the correct socket by OS
source "$(dirname "$0")/test-podman-setup.sh"

# Run Tests
# --------------------------
echo ""
echo "Running Podman E2E tests..."
E2E_START=$SECONDS

go test -cover -coverprofile=coverage.txt -tags e2e -timeout 30m ./e2e -v -run TestPodman_

E2E_DURATION=$((SECONDS - E2E_START))
E2E_MINS=$((E2E_DURATION / 60))
E2E_SECS=$((E2E_DURATION % 60))

echo "✓ Podman E2E tests completed successfully (duration: ${E2E_MINS}m ${E2E_SECS}s)"
