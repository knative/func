#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
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

connect() {
    echo 'Adding registry to hosts'
    echo "127.0.0.1 kind-registry" | sudo tee --append /etc/hosts
}

main "$@"

