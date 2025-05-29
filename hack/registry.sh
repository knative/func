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
# - Registers registry with Docker as trusted (Linux and macOS)
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
          if [[ "$(uname)" == "Darwin" ]]; then
            echo "If Docker Desktop was installed via Nix on macOS, you may need to manually configure the insecure registry."
            echo "Please confirm \"localhost:50000\" is specified as an insecure registry in the docker config file."
          else
            echo "If Docker was configured using nix, this command will fail to find daemon.json. please configure the insecure registry by modifying your nix config:"
            echo "  virtualisation.docker = {"
            echo "    enable = true;"
            echo "    daemon.settings.insecure-registries = [ \"localhost:50000\" ];"
            echo "  };"
          fi
        elif [ "$CONTAINER_ENGINE" == "podman" ]; then
          echo "${yellow}Warning: Nix detected${reset}"
          echo "If podman was configured via Nix, this command will likely fail.  At time of this writing, podman configured via the nix option 'virtualisation.podman' does not have an option for configuring insecure registries."
          echo "The configuration required is adding the following to registries.conf:"
          echo -e "  [[registry-insecure-local]]\n  location = \"localhost:50000\"\n  insecure = true"
        fi
    fi
}

set_registry_insecure() {
    # Determine the daemon.json location based on OS
    if [[ "$(uname)" == "Darwin" ]]; then
        # macOS: Docker Desktop stores daemon.json in ~/.docker/
        DAEMON_JSON="$HOME/.docker/daemon.json"
        USE_SUDO=""
    else
        # Linux: daemon.json is in /etc/docker/
        DAEMON_JSON="/etc/docker/daemon.json"
        USE_SUDO="sudo"
    fi

    # Create daemon.json if it doesn't exist
    if [ ! -f "$DAEMON_JSON" ]; then
        echo "{}" | $USE_SUDO tee "$DAEMON_JSON" > /dev/null
    fi

    # Update daemon.json with insecure registry
    patch=".\"insecure-registries\" = [\"localhost:50000\"]"
    $USE_SUDO jq "$patch" "$DAEMON_JSON" > /tmp/daemon.json.tmp && $USE_SUDO mv /tmp/daemon.json.tmp "$DAEMON_JSON"
    echo "OK $DAEMON_JSON"

    # Restart Docker based on OS
    if [[ "$(uname)" == "Darwin" ]]; then
        # macOS: Restart Docker Desktop
        echo "${yellow}*** If Docker Desktop is running, please restart it via the menu bar icon ***${reset}"
    else
        # Linux: Use service command
        sudo service docker restart
    fi
}

set_registry_insecure_podman() {
    FILE="/etc/containers/registries.conf"

    # Check if the section exists
    if ! sudo grep -q "\[\[registry-insecure-local\]\]" "$FILE"; then
        # Append the new section to the file
        echo -e "\n[[registry-insecure-local]]\nlocation = \"localhost:50000\"\ninsecure = true" | sudo tee -a "$FILE" > /dev/null
    fi

    # On macOS, set up SSH port forwarding so Podman VM can access host's localhost:50000
    if [[ "$(uname)" == "Darwin" ]]; then
        echo "Setting up port forwarding for Podman VM to access registry..."
        podman machine ssh -- -L 50000:localhost:50000 -N -f
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
