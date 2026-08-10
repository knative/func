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
# parallelization.
#
# This script presumes a local testing environment set up using:
#     func cluster create        - Creates test cluster with Knative, registry, etc.
#     func cluster create --dapr - Also installs Dapr (needed for some integration tests)
#     hack/gitlab.sh             - Installs GitLab in-cluster
#     hack/git-server.sh         - Starts a git server in-cluster
#
# Binaries (kubectl, kind, etc.) and kubeconfig are managed by
# `func cluster create` at $XDG_CONFIG_HOME/func/ (typically ~/.config/func/).
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

source "$(cd "$(dirname "$0")" && pwd)/common-func.sh"

# Enable Optional Tests
# ---------------------
# The defaults in the e2e test implementation are a bit more conservative.
# Here we toggle on All The Things.  Note that we still allow any settings
# made explicitly in the current environment to take precedence; just setting
# new defaults which are more expansive in testing scope.
export FUNC_CLUSTER_RETRIES="${FUNC_CLUSTER_RETRIES:-5}"
export FUNC_E2E_MATRIX="${FUNC_E2E_MATRIX:-true}"
export FUNC_E2E_VERBOSE="${FUNC_E2E_VERBOSE:-true}"
export FUNC_E2E_PODMAN="${FUNC_E2E_PODMAN:-true}"
export FUNC_E2E_CONFIG_CI="${FUNC_E2E_CONFIG_CI:-true}"
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
    test-e2e-config-ci

    completed
}

# -----
# Setup
# -----
setup() {
    # Determine script directory
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

    # KUBECONFIG is set by common-func.sh using func-managed paths:
    #   $XDG_CONFIG_HOME/func/clusters/{name}.local/kubeconfig.yaml

    # Point the E2E and integration tests at the managed kubeconfig and tools.
    # These tests deliberately ignore KUBECONFIG and installed bins, using
    # their own, with a fallback to the old hack/bin/.
    export FUNC_E2E_TOOLS="${FUNC_E2E_TOOLS:-${BIN_DIR}}"
    export FUNC_E2E_KUBECONFIG="${FUNC_E2E_KUBECONFIG:-${KUBECONFIG}}"
    export FUNC_INT_KUBECONFIG="${FUNC_INT_KUBECONFIG:-${KUBECONFIG}}"

    # GitLab test configuration
    export FUNC_TEST_GITLAB_PASS="${FUNC_TEST_GITLAB_PASS:-test-password-123}"

    # Generate a timestamp for use setting things which require uniqueness
    TIMESTAMP=$(date +%Y%m%d%H%M%S 2>/dev/null || date +%s 2>/dev/null || echo "$(date)")

    # Initialize coverage file
    echo "mode: atomic" >coverage.txt
}

# -------------
# Preconditions
# -------------
preconditions() {
    echo ""
    echo "${blue}Preconditions${reset}"

    # Check that kubectl is available (installed by func cluster create)
    if [[ -z "$KUBECTL" ]]; then
        echo "ERROR: kubectl not found"
        echo "Please run 'func cluster create' to set up a test cluster."
        exit 1
    fi

    # Check if cluster is allocated
    if [ ! -f "${KUBECONFIG}" ]; then
        echo "ERROR: KUBECONFIG not found at ${KUBECONFIG}"
        echo "Please run 'func cluster create' to set up a test cluster."
        exit 1
    fi

    # Verify cluster connectivity
    if ! $KUBECTL cluster-info &>/dev/null; then
        echo "ERROR: Cannot connect to Kubernetes cluster"
        echo "KUBECONFIG: ${KUBECONFIG}"
        echo "Please ensure your cluster is running and KUBECONFIG is valid."
        echo "You may need to run 'func cluster create'"
        exit 1
    fi

    # Check if GitLab is installed (if GitLab tests are enabled)
    if [ "${FUNC_INT_GITLAB_ENABLED}" = "1" ]; then
        if ! $KUBECTL get namespace gitlab &>/dev/null; then
            echo "ERROR: GitLab namespace not found"
            echo "Please run ./hack/gitlab.sh to install GitLab"
            exit 1
        fi
    fi

    # Check if Podman is installed and available (if Podman E2E tests are enabled)
    if [ "${FUNC_E2E_PODMAN}" = "true" ]; then
        if ! command -v podman >/dev/null 2>&1; then
            echo "ERROR: Podman is required for Podman E2E tests but not found!"
            echo "Please install Podman and ensure it is in your PATH."
            exit 1
        fi
    fi
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
}

# -------------------
# E2E CONFIG CI TESTS
# -------------------
# Mimics "test-e2e-config-ci" Workflow Job
# which sets:
#   FUNC_E2E_CONFIG_CI
test-e2e-config-ci() {
    echo ""
    echo "${blue}E2E - Config CI${reset}"
    make test-e2e-config-ci
    echo "${green}✓ E2E Config CI tests passed${reset}"
}

completed() {
    echo ""
    echo "${green}✅ Full test completed successfully${reset}"
}

main "$@"
