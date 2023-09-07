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

PIPELINE_ARTIFACTS="pkg/pipelines/resources/tekton/task/func-buildpacks/0.2/func-buildpacks.yaml pkg/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml"

function build_release() {
  echo "🚧 🐧 Building cross platform binaries: Linux 🐧 (amd64 / arm64 / ppc64le / s390x), MacOS 🍏, and Windows 🎠"
  FUNC_REPO_BRANCH_REF="$(git branch --show-current)" ETAG=${TAG} make cross-platform

  ARTIFACTS_TO_PUBLISH="func_darwin_amd64 func_darwin_arm64 func_linux_amd64 func_linux_arm64 func_linux_ppc64le func_linux_s390x func_windows_amd64.exe"
  ARTIFACTS_TO_PUBLISH="${ARTIFACTS_TO_PUBLISH} ${PIPELINE_ARTIFACTS}"
  sha256sum ${ARTIFACTS_TO_PUBLISH} > checksums.txt
  ARTIFACTS_TO_PUBLISH="${ARTIFACTS_TO_PUBLISH} checksums.txt"
  echo "🧮     Checksum:"
  cat checksums.txt
}

main $@
