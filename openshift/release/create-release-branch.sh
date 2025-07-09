#!/bin/bash

# Copyright 2019 The OpenShift Knative Authors
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

# Usage: create-release-branch.sh v0.4.1 release-0.4

set -exo pipefail

source openshift/release/common.sh

release=$1
target=$2

# Fetch the latest tags and checkout a new branch from the wanted tag.
git fetch upstream -v --tags
git checkout -b "$target" "$release"

# Copy the midstream specific files from the OPENSHIFT/main branch.
git fetch openshift main
# shellcheck disable=SC2086
git checkout openshift/main -- $MIDSTREAM_CUSTOM_FILES

openshift/release/apply-midstream-patches.sh

# Generate our OCP artifacts
tag=${target/release-/}
yq write --inplace openshift/project.yaml project.tag "knative-$tag"

git add .
git commit -m ":open_file_folder: Add OpenShift specific files"
