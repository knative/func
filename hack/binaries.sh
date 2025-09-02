#//usr/bin/env bash

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
# Installs binaries on linux systems.
#

source "$(dirname "$(realpath "$0")")/common.sh"

install_binaries() {
  assert_supported_os
  set_os_arch_vars
  warn_architecture

  local root="$(dirname "$(realpath "$0")")"
  local bin="${root}/bin"

  local kubectl_version=1.33.1
  local kind_version=0.29.0
  local dapr_version=1.14.1
  local helm_version=3.18.0
  local stern_version=1.32.0
  local kn_version=1.18.0
  local jq_version=1.7.1

  echo "${blue}Installing binaries${reset}"
  echo "  OS:           ${OS}"
  echo "  Architecture: ${ARCH}"
  echo "  Destination:  ${bin}"

  mkdir -p "${bin}"

  install_kubectl
  install_kind
  install_dapr
  install_helm
  install_stern
  install_kn
  install_jq

  echo "${green}DONE${reset}"

}

assert_supported_os() {
  os_name=$(uname -s)
  if [ "$os_name" != "Linux" ] && [ "$os_name" != "Darwin" ]; then
    echo "${yellow}----------------------------------------------------------------------${reset}"
    echo "${yellow}This script only supports Linux and Darwin (macOS)${reset}"
    echo "Please install the dependencies manually"
    echo "${yellow}----------------------------------------------------------------------${reset}"
    exit 1
  fi
}

set_os_arch_vars() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH_RAW=$(uname -m)
  
  # Map architecture names
  case "${ARCH_RAW}" in
    x86_64)
      ARCH="amd64"
      ;;
    aarch64|arm64)
      ARCH="arm64"
      ;;
    *)
      ARCH="${ARCH_RAW}"
      ;;
  esac
  
  # Override with environment variable if set
  ARCH="${ARCH:-$ARCH}"
}

warn_architecture() {
  arch=$(uname -m)
  if [ "$arch" != "x86_64" ] && [ "$arch" != "arm64" ] && [ "$arch" != "aarch64" ]; then
    echo -e "${yellow}Detected untested architecture ${arch}.${reset}\n This script is tested with amd64 and arm64, but you can use the ARCH env variable to specify an architecture to be interpolated in download links."
  fi
}

install_kubectl() {
    echo '=== kubectl'
    curl -sSLo "${bin}"/kubectl "https://dl.k8s.io/v${kubectl_version}/bin/${OS}/${ARCH}/kubectl"
    chmod +x "${bin}"/kubectl
    "${bin}"/kubectl version --client=true
}

install_kind() {
    echo '=== kind'
    curl -sSLo "${bin}"/kind "https://github.com/kubernetes-sigs/kind/releases/download/v$kind_version/kind-${OS}-${ARCH}"
    chmod +x "${bin}"/kind
    "${bin}"/kind version
}

install_dapr() {
    echo '=== dapr'
    curl -sSL "https://github.com/dapr/cli/releases/download/v$dapr_version/dapr_${OS}_${ARCH}.tar.gz" | \
      tar fxz - -C "${bin}" dapr
    "${bin}"/dapr version
}

install_helm() {
  echo '=== helm'
    curl -sSL "https://get.helm.sh/helm-v$helm_version-${OS}-${ARCH}.tar.gz" | \
      tar fxz - -C "${bin}" ${OS}-"${ARCH}"/helm
    mv "${bin}/${OS}-${ARCH}"/helm "${bin}" && rmdir "${bin}/${OS}-${ARCH}"
    "${bin}"/helm version
}

install_stern() {
  echo '=== stern'
  curl -sSL "https://github.com/stern/stern/releases/download/v${stern_version}/stern_${stern_version}_${OS}_${ARCH}.tar.gz" | \
    tar fxz - -C "${bin}" stern
  "${bin}"/stern -v
}

install_kn() {
  echo '=== kn'
  curl -sSLo "${bin}"/kn "https://github.com/knative/client/releases/download/knative-v${kn_version}/kn-${OS}-${ARCH}"
  chmod +x "${bin}"/kn
  "${bin}"/kn version
}

install_jq() {
  echo '=== jq'
  # jq uses different naming conventions for macOS
  if [ "$OS" = "darwin" ]; then
    JQ_OS="macos"
  else
    JQ_OS="linux"
  fi
  curl -sSLo "${bin}"/jq "https://github.com/jqlang/jq/releases/download/jq-${jq_version}/jq-${JQ_OS}-${ARCH}"
  chmod +x "${bin}"/jq
  "${bin}"/jq --version
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    install_binaries
  }
  main "$@"
fi
