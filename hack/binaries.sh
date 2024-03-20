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
  local kubectl_version=v1.29.2
  local kind_version=v0.22.0
  local dapr_version=v1.11.0
  local helm_version=v3.12.0
  local stern_version=1.25.0
  local kn_version=v1.13.0

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Fetching Binaries...${me}"

  install_kubectl
  install_kind
  install_yq
  install_dapr
  install_helm
  install_stern
  install_kn

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

install_stern() {
  echo 'Installing stern...'
  curl -sSL "https://github.com/stern/stern/releases/download/v${stern_version}/stern_${stern_version}_linux_amd64.tar.gz" | \
    tar fxz - -C /usr/local/bin/ stern
  stern -v
}

install_kn() {
  echo 'Installing kn...'
  curl -sSLo kn https://github.com/knative/client/releases/download/knative-${kn_version}/kn-linux-amd64
  chmod +x kn && sudo mv kn /usr/local/bin/
  kn version
}

main "$@"
