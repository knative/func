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
#

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "$(realpath "$0")")/common.sh"

output_file="${1:-cluster_log.txt}"

echo "::group::cluster events" >> "$output_file"
"$KUBECTL" get events -A >> "$output_file" 2>&1
echo "::endgroup::" >> "$output_file"

echo "::group::cluster containers logs" >> "$output_file"
if [[ -n "${STERN:-}" && -x "${STERN}" ]]; then
  "$STERN" '.*' --all-namespaces --no-follow >> "$output_file" 2>&1
else
  echo "stern not available, skipping container logs" >> "$output_file" 2>&1
fi
echo "::endgroup::" >> "$output_file"

