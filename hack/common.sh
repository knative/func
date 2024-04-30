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

init() {
  find_executables
  populate_environment
  define_colors
}

find_executables() {
  KUBECTL=$(find_executable "kubectl")
  KIND=$(find_executable "kind")
  DAPR=$(find_executable "dapr")
  HELM=$(find_executable "helm")
  STERN=$(find_executable "stern")
  KN=$(find_executable "kn")
  JQ=$(find_executable "jq")

  echo "Executables:"
  echo "  KUBECTL=${KUBECTL}"
  echo "  KIND=${KIND}"
  echo "  DAPR=${DAPR}"
  echo "  HELM=${HELM}"
  echo "  STERN=${STERN}"
  echo "  KN=${KN}"
  echo "  JQ=${JQ}"
}

populate_environment() {
  # User's KUBECOFNIG and that used by these scripts should be isolated:
  export KUBECONFIG="$(dirname "$(realpath "$0")")/bin/kubeconfig.yaml"
  export ARCH="${ARCH:-amd64}"
  export CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
  export TERM="${TERM:-dumb}"

  echo "Environment:"
  echo "  KUBECONFIG=${KUBECONFIG}"
  echo "  ARCH=${ARCH}"
  echo "  CONTAINER_ENGINE=${CONTAINER_ENGINE}"
  echo "  TERM=${TERM}"
}

define_colors() {
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

# find returns the path to an executable by name.
# An environment variable FUNC_TEST_$name takes precidence.
# Next is an executable matching the name in hack/bin/
# (the install location of hack/install-binaries.sh)
# Finally, a matching executable from the current PATH is used.
find_executable() {
  local name="$1" # requested binary name
  local path="" # the path to output

  # Use the environment variable if defined
  local env=$(echo "FUNC_TEST_$name" | awk '{print toupper($0)}')
  local path="${!env:-}"
  if [[ -x "$path" ]]; then
    echo "$path" & return 0
  fi

  # Use the binary installed into hack/bin/ by allocate.sh if
  # it exists.
  path=$(dirname "$(realpath "$0")")"/bin/$name"
  if [[ -x "$path" ]]; then
    echo "$path" & return 0
  fi

  # Finally fallback to anything matchin in the current PATH
  path=$(command -v "$name")
  if [[ -x "$path" ]]; then
    echo "$path" & return 0
  fi

  echo "Error: ${name} not found." >&2
  return 1
}

init "$@"
