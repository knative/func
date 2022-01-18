#!/usr/bin/env bash

# 
# - Registers registry with Docker as trusted (linux only)
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

  echo "${em}DONE${me}"

}

set_registry_insecure() {
    echo 'Setting registry as trusted local-only'
    patch=".\"insecure-registries\" = [\"localhost:50000\""]
    sudo jq "$patch" /etc/docker/daemon.json > /tmp/daemon.json.tmp && sudo mv /tmp/daemon.json.tmp /etc/docker/daemon.json
    sudo service docker restart
}

main "$@"

