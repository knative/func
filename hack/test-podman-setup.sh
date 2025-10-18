#!/usr/bin/env bash

# Podman Setup Script
#
# Configures FUNC_E2E_PODMAN_HOST environment variable for Podman tests.
# This script detects the correct Podman socket path based on the operating system
# and Podman configuration.
#
# Environment Variables:
#   FUNC_E2E_PODMAN: Set to "true" to enable Podman configuration (default: false)
#   FUNC_E2E_PODMAN_HOST: Will be set to the detected Podman socket path
#
# Exit Codes:
#   0: Success (or FUNC_E2E_PODMAN not enabled)
#   1: Podman not installed
#   2: Podman service not running
#   3: Could not determine socket path

set -o errexit
set -o nounset
set -o pipefail

source "$(cd "$(dirname "$0")" && pwd)/common.sh"

# Only run setup if FUNC_E2E_PODMAN is true
if [ "${FUNC_E2E_PODMAN:-false}" != "true" ]; then
    exit 0
fi

# Check if Podman is available
if ! command -v podman &>/dev/null; then
    echo "${red}ERROR: Podman not found${reset}" >&2
    echo "Please install Podman to run Podman tests" >&2
    echo "Installation instructions:" >&2
    echo "  - macOS: brew install podman" >&2
    echo "  - Linux: See https://podman.io/getting-started/installation" >&2
    echo "  - Windows: Download from https://github.com/containers/podman/releases" >&2
    exit 1
fi

# Check if Podman service is running
if ! podman info &>/dev/null; then
    echo "${red}ERROR: Podman service is not running${reset}" >&2
    echo "Please start Podman service with: podman system service --time=0 &" >&2
    exit 2
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
        if [ -n "${DOCKER_HOST:-}" ]; then
            export FUNC_E2E_PODMAN_HOST="${DOCKER_HOST}"
        else
            echo "${yellow}Warning: Could not determine Podman socket path${reset}" >&2
            echo "Try running: podman machine stop && podman machine start" >&2
            echo "Then use the DOCKER_HOST value it provides" >&2
            PODMAN_SOCKET="$(podman info -f '{{.Host.RemoteSocket.Path}}')"
            if [ -z "${PODMAN_SOCKET}" ]; then
                echo "${red}ERROR: Could not determine Podman socket path${reset}" >&2
                exit 3
            fi
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

echo ""
echo "${green}✓ Podman socket: ${FUNC_E2E_PODMAN_HOST}${reset}"
