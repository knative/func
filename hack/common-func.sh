# Common utilities for scripts that use `func cluster create` managed paths.
#
# This is the successor to common.sh for scripts that work with the new
# func-managed cluster infrastructure (binaries at $XDG_CONFIG_HOME/func/bin,
# kubeconfig at $XDG_CONFIG_HOME/func/clusters/{name}.local/kubeconfig.yaml).
#
# Scripts that still use the old hack/bin/ paths should continue to source
# common.sh instead.

init() {
  set_func_paths
  find_executables
  define_colors
}

# set_func_paths configures FUNC_CONFIG_DIR, BIN_DIR, and KUBECONFIG
# using the same path logic as pkg/cluster/config.go.
set_func_paths() {
  local cluster_name="${FUNC_CLUSTER_NAME:-func}"

  if [[ -n "${XDG_CONFIG_HOME:-}" ]]; then
    FUNC_CONFIG_DIR="${XDG_CONFIG_HOME}/func"
  else
    FUNC_CONFIG_DIR="${HOME}/.config/func"
  fi

  BIN_DIR="${FUNC_CONFIG_DIR}/bin"
  export KUBECONFIG="${FUNC_CONFIG_DIR}/clusters/${cluster_name}.local/kubeconfig.yaml"

  # Detect architecture
  if [[ -z "${ARCH:-}" ]]; then
    local machine_arch
    machine_arch=$(uname -m)
    case $machine_arch in
      x86_64) export ARCH="amd64" ;;
      aarch64|arm64) export ARCH="arm64" ;;
      *) export ARCH="amd64" ;;
    esac
  else
    export ARCH="$ARCH"
  fi

  export CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
  export TERM="${TERM:-dumb}"

  echo "FUNC_CONFIG_DIR=${FUNC_CONFIG_DIR}"
  echo "BIN_DIR=${BIN_DIR}"
  echo "KUBECONFIG=${KUBECONFIG}"
  echo "CONTAINER_ENGINE=${CONTAINER_ENGINE}"
}

# find_executables locates kubectl (the only host-side tool the test scripts
# shell out to; all other tools are invoked through the `func` binary).
# Fallback order: FUNC_TEST_KUBECTL > $FUNC_CONFIG_DIR/bin/kubectl > PATH.
find_executables() {
  KUBECTL=$(find_executable "kubectl" || true)
}

find_executable() {
  local name="$1"
  local path=""

  # 1. Environment variable override
  local env
  env=$(echo "FUNC_TEST_$name" | awk '{print toupper($0)}')
  path="${!env:-}"
  if [[ -x "$path" ]]; then
    echo "$path"; return 0
  fi

  # 2. func-managed bin directory
  path="${BIN_DIR}/${name}"
  if [[ -x "$path" ]]; then
    echo "$path"; return 0
  fi

  # 3. System PATH
  path=$(command -v "$name")
  if [[ -x "$path" ]]; then
    echo "$path"; return 0
  fi

  echo "Warning: ${name} not found." >&2
  return 1
}

define_colors() {
  local TERM="$TERM"
  if [[ -z "$TERM" || "$TERM" == "dumb" ]]; then
    TERM="xterm"
  fi
  # shellcheck disable=SC2155
  red=$(tput bold)$(tput setaf 1)
  # shellcheck disable=SC2155
  green=$(tput bold)$(tput setaf 2)
  # shellcheck disable=SC2155
  blue=$(tput bold)$(tput setaf 4)
  # shellcheck disable=SC2155
  grey=$(tput bold)$(tput setaf 8)
  # shellcheck disable=SC2155
  yellow=$(tput bold)$(tput setaf 11)
  # shellcheck disable=SC2155
  reset=$(tput sgr0)
}

init "$@"
