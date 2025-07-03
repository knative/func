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
# Dump cluster logs for debugging purposes
# This script is designed to be fail-soft - errors won't cause script failure
#

set -o nounset
set -o pipefail

source "$(dirname "$(realpath "$0")")/common.sh"

output_file="${1:-cluster_log.txt}"

# Cluster events
echo "::group::cluster events" >> "$output_file" 2>&1 || true
if "$KUBECTL" get events -A >> "$output_file" 2>&1; then
  echo "Successfully captured cluster events" >> "$output_file" 2>&1 || true
else
  echo "Warning: Failed to capture cluster events (exit code: $?)" >> "$output_file" 2>&1 || true
fi
echo "::endgroup::" >> "$output_file" 2>&1 || true

# Container logs
echo "::group::cluster containers logs" >> "$output_file" 2>&1 || true
if [[ -n "${STERN:-}" && -x "${STERN}" ]]; then
  if "$STERN" '.*' --all-namespaces --no-follow >> "$output_file" 2>&1; then
    echo "Successfully captured container logs with stern" >> "$output_file" 2>&1 || true
  else
    echo "Warning: stern failed to capture container logs (exit code: $?)" >> "$output_file" 2>&1 || true
  fi
else
  echo "stern not available, skipping container logs" >> "$output_file" 2>&1 || true
fi
echo "::endgroup::" >> "$output_file" 2>&1 || true

# Always exit successfully
exit 0

