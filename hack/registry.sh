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

source "$(cd "$(dirname "$0")" && pwd)/common.sh"

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
        # Linux: Skip restart to avoid breaking func-registry container
        echo "Skipping Docker restart on Linux (daemon.json updated, restart will break func-registry)"
    fi
}

set_registry_insecure_podman() {
    # Handle both rootful and rootless Podman configurations
    USER_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/containers"
    USER_CONFIG_FILE="$USER_CONFIG_DIR/registries.conf"
    SYSTEM_CONFIG_FILE="/etc/containers/registries.conf"
    CONFIG_FILE=""
    NEED_SUDO=false

    # Determine which config file to use
    if [ -f "$USER_CONFIG_FILE" ]; then
        # User-level config exists, use it
        CONFIG_FILE="$USER_CONFIG_FILE"
        echo "Using existing user Podman registry config at $CONFIG_FILE"
    elif [ -f "$SYSTEM_CONFIG_FILE" ]; then
        # System-level config exists, use it (will need sudo)
        CONFIG_FILE="$SYSTEM_CONFIG_FILE"
        NEED_SUDO=true
        echo "Using existing system Podman registry config at $CONFIG_FILE"
    else
        # Neither config exists - Podman may be installed but unconfigured
        # Create a user-level config with v2 format
        echo "No existing Podman registries.conf found."

        # Try to create user-level config directory
        if mkdir -p "$USER_CONFIG_DIR" 2>/dev/null; then
            CONFIG_FILE="$USER_CONFIG_FILE"
            echo "Creating new user-level Podman registry config at $CONFIG_FILE"
            # Create new v2 format config
            echo "# Podman registries configuration" > "$CONFIG_FILE"
            echo "# Generated by func registry.sh" >> "$CONFIG_FILE"
            echo "" >> "$CONFIG_FILE"
            echo "[[registry]]" >> "$CONFIG_FILE"
            echo "location = \"localhost:50000\"" >> "$CONFIG_FILE"
            echo "insecure = true" >> "$CONFIG_FILE"
            echo "Successfully created Podman registry configuration for localhost:50000"
            return 0
        else
            echo "Could not create user config directory. Skipping Podman registry configuration."
            echo "Note: Podman can still work with fully qualified image names."
            return 0
        fi
    fi

    # Detect config format (v1 or v2)
    IS_V2_FORMAT=false
    if [ "$NEED_SUDO" = true ]; then
        if sudo grep -q "^\[\[registry\]\]" "$CONFIG_FILE" 2>/dev/null; then
            IS_V2_FORMAT=true
        fi
    else
        if grep -q "^\[\[registry\]\]" "$CONFIG_FILE" 2>/dev/null; then
            IS_V2_FORMAT=true
        fi
    fi

    # Check if localhost:50000 is already configured
    ALREADY_CONFIGURED=false
    if [ "$NEED_SUDO" = true ]; then
        if sudo grep -q "localhost:50000" "$CONFIG_FILE" 2>/dev/null; then
            ALREADY_CONFIGURED=true
        fi
    else
        if grep -q "localhost:50000" "$CONFIG_FILE" 2>/dev/null; then
            ALREADY_CONFIGURED=true
        fi
    fi

    if [ "$ALREADY_CONFIGURED" = true ]; then
        echo "localhost:50000 is already configured in $CONFIG_FILE"
        return 0
    fi

    # Add the configuration in the appropriate format
    if [ "$IS_V2_FORMAT" = true ]; then
        # Use v2 format
        echo "Adding localhost:50000 as insecure registry (v2 format)"
        if [ "$NEED_SUDO" = true ]; then
            echo -e "\n[[registry]]\nlocation = \"localhost:50000\"\ninsecure = true" | sudo tee -a "$CONFIG_FILE" > /dev/null
        else
            echo -e "\n[[registry]]\nlocation = \"localhost:50000\"\ninsecure = true" >> "$CONFIG_FILE"
        fi
    else
        # Use v1 format
        echo "Adding localhost:50000 as insecure registry (v1 format)"
        if [ "$NEED_SUDO" = true ]; then
            # Check if [registries.insecure] section exists
            if sudo grep -q "^\[registries.insecure\]" "$CONFIG_FILE" 2>/dev/null; then
                # Section exists, add to the registries list
                sudo sed -i '/^\[registries.insecure\]/a registries = ["localhost:50000"]' "$CONFIG_FILE"
            else
                # Section doesn't exist, add it
                echo -e "\n[registries.insecure]\nregistries = [\"localhost:50000\"]" | sudo tee -a "$CONFIG_FILE" > /dev/null
            fi
        else
            # Check if [registries.insecure] section exists
            if grep -q "^\[registries.insecure\]" "$CONFIG_FILE" 2>/dev/null; then
                # Section exists, add to the registries list
                sed -i '/^\[registries.insecure\]/a registries = ["localhost:50000"]' "$CONFIG_FILE"
            else
                # Section doesn't exist, add it
                echo -e "\n[registries.insecure]\nregistries = [\"localhost:50000\"]" >> "$CONFIG_FILE"
            fi
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
