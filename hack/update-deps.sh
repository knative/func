#!/usr/bin/env bash

# Copyright 2021 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

source "$(go run knative.dev/hack/cmd/script library.sh)"

go_update_deps "$@"

echo ">> args='$@'"
# This is a guard for running only when its running via bot to update deps and
# potentially create a PR.
# When this is running in 'verify deps' workflow it runs with no arguments.
# We don't want these changes to break the GH Action for all PRs therefore we
# limit this only for dependency bumps.
if [[ $# -gt 0 ]]; then
    echo ">> Running make hack-generate-components"
    # Update hack components
    make hack-generate-components
fi
