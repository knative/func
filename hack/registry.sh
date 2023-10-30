#!/usr/bin/env bash

# 
# - Registers registry with Docker as trusted (linux only)
#

set -o errexit
set -o nounset
set -o pipefail

CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
export TERM="${TERM:-dumb}"

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Configuring for CI...${me}"


  # Check the value of CONTAINER_ENGINE
  echo 'Setting registry as trusted local-only'
  if [ "$CONTAINER_ENGINE" == "docker" ]; then
      set_registry_insecure
  elif [ "$CONTAINER_ENGINE" == "podman" ]; then
      set_registry_insecure_podman
  fi

  echo "${em}DONE${me}"

}

set_registry_insecure() {
    patch=".\"insecure-registries\" = [\"localhost:50000\""]
    sudo jq "$patch" /etc/docker/daemon.json > /tmp/daemon.json.tmp && sudo mv /tmp/daemon.json.tmp /etc/docker/daemon.json
    sudo service docker restart
}

set_registry_insecure_podman() {
    FILE="/etc/containers/registries.conf"

    # Check if the section exists
    if ! sudo grep -q "\[\[registry-insecure-local\]\]" "$FILE"; then
        # Append the new section to the file
        echo -e "\n[[registry-insecure-local]]\nlocation = \"localhost:50000\"\ninsecure = true" | sudo tee -a "$FILE" > /dev/null
    fi
}

main "$@"

