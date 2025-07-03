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
# Update bash and install GNU sed on macOS if needed
#

set -o errexit
set -o nounset
set -o pipefail

main() {
  define_colors

  # Only run on macOS
  if [[ "$(uname)" != "Darwin" ]]; then
    echo "${blue}Not running on macOS, skipping bash update${reset}"
    exit 0
  fi

  echo "${blue}Updating bash and GNU tools on macOS...${reset}"

  # Check if Homebrew is installed, install if not
  if ! command -v brew >/dev/null 2>&1; then
    echo "${blue}Installing Homebrew...${reset}"
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  fi

  # Update Homebrew
  echo "${blue}Updating Homebrew...${reset}"
  brew update

  # Install bash 4+ (macOS ships with bash 3.x)
  echo "${blue}Installing bash...${reset}"
  brew install bash

  # Install GNU sed (macOS ships with BSD sed)
  echo "${blue}Installing GNU sed...${reset}"
  brew install gnu-sed

  # For GitHub Actions, add to PATH
  if [[ -n "${GITHUB_PATH:-}" ]]; then
    echo "/usr/local/bin" >> "$GITHUB_PATH"
    echo "$(brew --prefix)/opt/gnu-sed/libexec/gnubin" >> "$GITHUB_PATH"
  else
    # For local development, provide instructions
    echo ""
    echo "${green}✅ Bash and GNU sed installed${reset}"
    echo ""
    echo "${yellow}To use the updated tools, add these to your PATH:${reset}"
    echo "  export PATH=\"/usr/local/bin:\$PATH\""
    echo "  export PATH=\"$(brew --prefix)/opt/gnu-sed/libexec/gnubin:\$PATH\""
  fi
}

define_colors() {
  export TERM="${TERM:-dumb}"

  # For some reason TERM=dumb results in the tput commands exiting 1.  It must
  # not support that terminal type. A reasonable fallback should be "xterm".
  local TERM="$TERM"
  if [[ -z "$TERM" || "$TERM" == "dumb" ]]; then
    TERM="xterm" # Set TERM to a tput-friendly value when undefined or "dumb".
  fi
  # shellcheck disable=SC2155
  red=$(tput bold)$(tput setaf 1)
  # shellcheck disable=SC2155
  green=$(tput bold)$(tput setaf 2)
  # shellcheck disable=SC2155
  blue=$(tput bold)$(tput setaf 4)
  # shellcheck disable=SC2155
  grey=$(tput bold)$(tput setaf 8)
  # shellcheck disable=SC2155
  yellow=$(tput bold)$(tput setaf 11)
  # shellcheck disable=SC2155
  reset=$(tput sgr0)
}


main "$@"
