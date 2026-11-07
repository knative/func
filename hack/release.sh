#!/usr/bin/env bash

# Copyright 2019 The Knative Authors
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

# Documentation about this script and how to use it can be found
# at https://github.com/knative/hack
ORG_NAME=knative
VALIDATION_TESTS="make test"

source "$(go run knative.dev/hack/cmd/script release.sh)"

# A vX.Y.Z release must embed the func-utils image pinned for X.Y, and that
# image must exist. Both hold when release-X.Y is cut from main: main embeds
# the minor in development and its green pushes publish that image (see
# docs/func-utils-image.md).
function check_func_utils_image() {
  local want actual
  want="ghcr.io/knative/func-utils:$(major_minor_version "$1")"
  actual="$(make -s --no-print-directory func-utils-image)"
  [[ "${actual}" == "${want}" ]] \
    || abort "release $1 would embed func-utils image '${actual}', want '${want}'; see docs/func-utils-image.md"
  go run github.com/google/go-containerregistry/cmd/crane digest "${want}" > /dev/null \
    || abort "func-utils image '${want}' does not exist"
  echo "🧰 func-utils image: ${want}"
}

function build_release() {
  echo "🚧 🐧 Building cross platform binaries: Linux 🐧 (amd64 / arm64 / ppc64le / s390x), MacOS 🍏, and Windows 🎠"

  local go_module_version
  local knative_version
  if (( TAG_RELEASE )); then
    knative_version="${TAG}"
    # Check if TAG is a nightly date-based tag (vYYYYMMDD-hash) vs semver (knative-vX.Y.Z)
    if [[ "${TAG}" =~ ^v[0-9]{8}- ]]; then
      # Nightly build: use TAG directly as version
      go_module_version="${TAG}"
    else
      # Release build: convert knative version to go module version
      go_module_version="v0.$(( $(minor_version "$TAG") + 27 )).$(patch_version "$TAG")"
      check_func_utils_image "${TAG}"
    fi
  else
    knative_version="$(git describe --tags --match 'knative-*')"
    go_module_version="$(git describe --tags --match 'v*')"
  fi
  VERS="${go_module_version}" KVER="${knative_version}" make cross-platform

  ARTIFACTS_TO_PUBLISH="func_darwin_amd64 func_darwin_arm64 func_linux_amd64 func_linux_arm64 func_linux_ppc64le func_linux_s390x func_windows_amd64.exe"
  ARTIFACTS_TO_PUBLISH="${ARTIFACTS_TO_PUBLISH}"
  sha256sum ${ARTIFACTS_TO_PUBLISH} > checksums.txt
  ARTIFACTS_TO_PUBLISH="${ARTIFACTS_TO_PUBLISH} checksums.txt"
  echo "🧮     Checksum:"
  cat checksums.txt
}

main $@
