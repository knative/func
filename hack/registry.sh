#!/usr/bin/env bash

# 
# Set up local registry (linux only)
# - Registers registry with Docker as trusted
# - Adds 'kind-rgistry' to /etc/hosts
#

set -o errexit
set -o nounset
set -o pipefail

export TERM="${TERM:-dumb}"

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Configuring for CI...${me}"

  set_registry_insecure
  patch_hosts

  echo "${em}DONE${me}"

}

set_registry_insecure() {
    echo 'Setting registry as trusted local-only'
    patch=".\"insecure-registries\" = [\"kind-registry:5000\""]
    sudo jq "$patch" /etc/docker/daemon.json > /tmp/daemon.json.tmp && sudo mv /tmp/daemon.json.tmp /etc/docker/daemon.json
    sudo service docker restart
}

patch_hosts() {
    echo 'Adding registry to hosts'
    echo "127.0.0.1 kind-registry" | sudo tee --append /etc/hosts
}

main "$@"

