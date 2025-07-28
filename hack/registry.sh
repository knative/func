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

  # Configure both Docker and Podman if they exist
  # This supports environments where both are installed
  echo 'Setting registry as trusted local-only'
  
  # Try to configure Docker if it exists
  if command -v docker &> /dev/null; then
      echo "Configuring Docker for insecure registry..."
      set_registry_insecure || echo "${yellow}Warning: Failed to configure Docker${reset}"
  fi
  
  # Try to configure Podman if it exists
  if command -v podman &> /dev/null; then
      echo "Configuring Podman for insecure registry..."
      set_registry_insecure_podman || echo "${yellow}Warning: Failed to configure Podman${reset}"
  fi

  echo "${green}✅ Registry${reset}"
}

warn_nix() {
    if [[ -x $(command -v "nix") || -x $(command -v "nixos-rebuild") ]]; then
        echo "${yellow}Warning: Nix detected${reset}"
        
        # Warn about Docker if it's installed
        if command -v docker &> /dev/null; then
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
        fi
        
        # Warn about Podman if it's installed
        if command -v podman &> /dev/null; then
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
    # Handle both rootful and rootless Podman configurations
    # For rootless, use user's config directory
    USER_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/containers"
    SYSTEM_CONFIG_FILE="/etc/containers/registries.conf"
    
    # Try user config first (rootless)
    if [ -w "$USER_CONFIG_DIR" ] || mkdir -p "$USER_CONFIG_DIR" 2>/dev/null; then
        USER_CONFIG_FILE="$USER_CONFIG_DIR/registries.conf"
        echo "Configuring rootless Podman registry at $USER_CONFIG_FILE"
        
        # Create the file if it doesn't exist
        if [ ! -f "$USER_CONFIG_FILE" ]; then
            echo "" > "$USER_CONFIG_FILE"
        fi
        
        # Check if the section exists
        if ! grep -q "\[\[registry\]\]" "$USER_CONFIG_FILE" || ! grep -A2 "\[\[registry\]\]" "$USER_CONFIG_FILE" | grep -q "location.*localhost:50000"; then
            # Append the new section to the file
            echo -e "\n[[registry]]\nlocation = \"localhost:50000\"\ninsecure = true" >> "$USER_CONFIG_FILE"
        fi
    fi
    
    # Also try system config if we have sudo (rootful)
    if command -v sudo &> /dev/null && sudo -n true 2>/dev/null; then
        echo "Configuring system-wide Podman registry at $SYSTEM_CONFIG_FILE"
        # Check if the section exists
        if ! sudo grep -q "\[\[registry\]\].*localhost:50000" "$SYSTEM_CONFIG_FILE" 2>/dev/null; then
            # Append the new section to the file
            echo -e "\n[[registry]]\nlocation = \"localhost:50000\"\ninsecure = true" | sudo tee -a "$SYSTEM_CONFIG_FILE" > /dev/null
        fi
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
