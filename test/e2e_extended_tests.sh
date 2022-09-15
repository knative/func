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
# Runs E2E additional set of tests against kn func cli.
# By default it will run e2e tests against 'func' binary, but you can change it to use 'kn func' instead
#
# The following environment variable can be set in order to customize e2e execution:
#
# E2E_USE_KN_FUNC    When set to "true" indicates e2e to issue func command using kn cli.
#
# E2E_REGISTRY_URL   Indicates a specific registry (i.e: "quay.io/user") should be used. Make sure
#                    to authenticate to the registry (i.e: docker login ...) prior to execute the script
#
# E2E_FUNC_BIN_PATH  Path to func binary. Derived by this script
#

set -o errexit
set -o nounset
set -o pipefail

export SOCAT_IMAGE="quay.io/boson/alpine-socat:1.7.4.3-r1-non-root"

use_kn_func=${E2E_USE_KN_FUNC:-}

curdir=$(pwd)
cd $(dirname $0)
cd ../

# Make sure 'func' binary is built in case KN FUNC was not required for testing
if [[ ! -f func && "$use_kn_func" != "true" ]]; then
  echo "func binary not found. Please run 'make build' prior to run e2e."
  exit 1
fi

export E2E_FUNC_BIN_PATH=$(pwd)/func

go clean -testcache
go test -v -test.v -tags="e2e" ./test/_e2e/
ret=$?

cd $curdir
exit $ret
