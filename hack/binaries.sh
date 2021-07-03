#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
  local kubectl_version=v1.21.2
  local kind_version=v0.11.1

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Fetching Binaries...${me}"

  kubectl
  kind
  yq

  echo "${em}DONE${me}"

}

kubectl() {
    echo 'Installing kubectl...'
    curl -sSLO "https://storage.googleapis.com/kubernetes-release/release/$kubectl_version/bin/linux/amd64/kubectl"
    chmod +x kubectl
    sudo mv kubectl /usr/local/bin/kubectl
}

kind() {
    echo 'Installing kind...'
    curl -sSLo kind "https://github.com/kubernetes-sigs/kind/releases/download/$kind_version/kind-linux-amd64"
    chmod +x kind
    sudo mv kind /usr/local/bin/kind
}

yq() {
    echo 'Installing yq...'
    pip3 install yq
}


main "$@"
