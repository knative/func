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
# By running `make test-full`, this script intends to roughly
# replicate what is run in GitHub CI, but locally and without
# parellelization.
#
# This script presumes a local testing environment set up using the
# helper scripts in ./hack and performs some precondition checks to ensure
# resources are available for the features enabled (nonexhaustive).
#     hack/binaries.sh   - Installs necessary binaries in ./hack/bin
#     hack/cluster.sh    - Start test cluster with Knative Serving/Eventing
#     hack/registry.sh   - Starts and configures a local container registry
#     hack/gitlab.sh     - Installs GitLab in-cluster
#     hack/git-server.sh - Starts a git server in-cluster
#
# Also note that when run with all default options, the "Matrix"
# test will run, requiring that all supported language toolchains are
# also available.
#
# For more targeted E2E testing without all the bells-and-whistles,
# see the e2e/e2e_test.go file which can have it's tests run directly.
#
# make test-full 2>&1 | tee ./test-full.log

set -o errexit
set -o nounset
set -o pipefail

source "$(cd "$(dirname "$0")" && pwd)/common.sh"

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

main() {
    setup         # paths etc
    preconditions # binaries exist, cluster available etc

    # 1:1 for GitHub Workflow Jobs of the same name:
    precheck
    test-unit
    test-templates
    test-integration
    test-e2e
    test-e2e-podman
    test-e2e-runtimes
}

# -----
# Setup
# -----
setup() {
    # Determine script directory
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

    # Set PATH and KUBECONFIG
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
}

# -------------
# Preconditions
# -------------
preconditions() {
    echo ""
    echo "${blue}Preconditions${reset}"

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
    if [ "${FUNC_INT_GITLAB_ENABLED}" = "1" ]; then
        if ! kubectl get namespace gitlab &>/dev/null; then
            echo "ERROR: GitLab namespace not found"
            echo "Please run ./hack/gitlab.sh to install GitLab"
            exit 1
        fi
    fi

    # TODO: if Podman tests are enabled, check that podman is installed and running

    echo ""
    echo "${green}✓ Preconditions checks passed${reset}"
}

# --------
# PRECHECK
# --------
# Mimics "precheck" Workflow Job
precheck() {
    echo ""
    echo "${blue}Precheck${reset}"

    make check
    echo "${green}- Code check passed (make check)${reset}"
    make check-schema
    echo "${green}- Schema check passed (make check-schema)${reset}"
    make check-templates
    echo "${green}- Templates check passed (make check-templates${reset}"
    make check-embedded-fs
    echo "${green}- Embedded filesystem check passed (make check-embedded-fs)${reset}"

    echo "${green}✓ Code checks passed${reset}"
}

# ----------
# UNIT TESTS
# ----------
# Mimics "test-unit" Workflow Job
test-unit() {
    echo ""
    echo "${blue}Unit Tests${reset}"
    make test
    echo "${green}✓ Unit tests passed${reset}"
}

# --------------
# TEMPLATE TESTS
# --------------
# Mimics "test-templates" Workflow Job
test-templates() {
    echo ""
    echo "${blue}Template Tests${reset}"
    make test-templates
    echo "${green}✓ Template tests passed${reset}"
}

# -----------------
# INTEGRATION TESTS
# -----------------
# Mimics "test-integration" Workflow Job
# which sets:
#   FUNC_INT_TEKTON_ENABLED
#   FUNC_INT_GITLAB_ENABLED
#   FUNC_INT_GITLAB_HOSTNAME
#   FUNC_INT_PAC_HOST
test-integration() {
    echo ""
    echo "${blue}Integration Tests${reset}"
    make test-templates
    echo "${green}✓ Integration tests passed${reset}"
}

# ---------
# E2E TESTS
# ---------
# Mimics "test-e2e" Workflow Job
# see e2e/e2e_test.go for available ENV option
test-e2e() {
    echo ""
    echo "${blue}E2E - Core, Metadata, and Remote${reset}"
    make test-e2e
    echo "${green}✓ E2E tests passed (Core, Metadata, Remote)${reset}"
}

# ----------------
# E2E PODMAN TESTS
# ----------------
# Mimics "test-e2e-podman" Workflow Job
# which sets:
#   FUNC_E2E_PODMAN
test-e2e-podman() {
    echo ""
    echo "${blue}E2E - Podman${reset}"
    make test-e2e-podman
    echo "${green}✓ E2E Podman tests passed${reset}"
}

# -----------------
# E2E RUNTIME TESTS
# -----------------
# Mimics "test-e2e-runtimes" Workflow Job
# which sets:
#   FUNC_E2E_MATRIX
test-e2e-runtimes() {
    echo ""
    echo "${blue}E2E - Runtimes${reset}"
    make test-e2e-matrix
    echo "${green}✓ E2E Runtime tests passed${reset}"

    echo ""
    echo "${green}✅ Full test completed successfully${reset}"
}

main "$@"
