#!/usr/bin/env bash
#
# Installs binaries on linux systems.
#
# Note that there are multiple 'yq's out there.  The one we want is kislyuk/yq,
# which is a thin wrapper around jq.

set -o errexit
set -o nounset
set -o pipefail

export TERM="${TERM:-dumb}"

main() {
  local kubectl_version=v1.27.2
  local kind_version=v0.19.0
  local dapr_version=v1.10.0
  local helm_version=v3.12.0

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Fetching Binaries...${me}"

  install_kubectl
  install_kind
  install_yq
  install_dapr
  install_helm

  echo "${em}DONE${me}"

}

install_kubectl() {
    echo 'Installing kubectl...'
    curl -sSLO "https://storage.googleapis.com/kubernetes-release/release/$kubectl_version/bin/linux/amd64/kubectl"
    chmod +x kubectl
    sudo mv kubectl /usr/local/bin/
    kubectl version --client=true
}

install_kind() {
    echo 'Installing kind...'
    curl -sSLo kind "https://github.com/kubernetes-sigs/kind/releases/download/$kind_version/kind-linux-amd64"
    chmod +x kind
    sudo mv kind /usr/local/bin/
    kind version
}

install_yq() {
    echo 'Installing yq...'
    sudo pip3 install yq
    yq --version
}

install_dapr() {
    echo 'Installing dapr...'
    curl -sSLo dapr.tgz "https://github.com/dapr/cli/releases/download/$dapr_version/dapr_linux_amd64.tar.gz"
    tar -xvf dapr.tgz
    chmod +x dapr
    sudo mv dapr /usr/local/bin/
    dapr version
}

install_helm() {
  echo 'Installing helm (v3)...'
    curl -sSLo helm.tgz "https://get.helm.sh/helm-$helm_version-linux-amd64.tar.gz"
    tar -xvf helm.tgz
    chmod +x linux-amd64/helm
    sudo mv linux-amd64/helm /usr/local/bin/
    helm version
}

main "$@"
