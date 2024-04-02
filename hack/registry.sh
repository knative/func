#!/usr/bin/env bash

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
# - Registers registry with Docker as trusted (linux only)
#

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "$(realpath "$0")")/common.sh"

registry() {
  echo "${blue}Enabling Insecure Local Registry${reset}"

  warn_nix

  # Check the value of CONTAINER_ENGINE
  echo 'Setting registry as trusted local-only'
  if [ "$CONTAINER_ENGINE" == "docker" ]; then
      set_registry_insecure
  elif [ "$CONTAINER_ENGINE" == "podman" ]; then
      set_registry_insecure_podman
  fi

  echo "${green}✅ Registry${reset}"
}

warn_nix() {
    if [[ -x $(command -v "nix") || -x $(command -v "nixos-rebuild") ]]; then
        if [ "$CONTAINER_ENGINE" == "docker" ]; then
          echo "${yellow}Warning: Nix detected${reset}"
          echo "If Docker was configured using nix, this command will fail to find daemon.json. please configure the insecure registry by modifying your nix config:"
          echo "  virtualisation.docker = {"
          echo "    enable = true;"
          echo "    daemon.settings.insecure-registries = [ \"localhost:50000\" ];"
          echo "  };"
        elif [ "$CONTAINER_ENGINE" == "podman" ]; then
          echo "${yellow}Warning: Nix detected${reset}"
          echo "If podman was configured via Nix, this command will likely fail.  At time of this writing, podman configured via the nix option 'virtualisation.podman' does not have an option for configuring insecure registries."
          echo "The configuration required is adding the following to registries.conf:"
          echo -e "  [[registry-insecure-local]]\n  location = \"localhost:50000\"\n  insecure = true"
        fi
    fi
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

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    registry
  }
  main "$@"
fi
