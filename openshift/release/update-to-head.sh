#!/usr/bin/env bash

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

# The local git repo must have a remote "upstream" pointing
# to knative-sandbox/kn-plugin-func, and a remote "openshift"
# pointing to openshift-knative/kn-plugin-func.

# Synchs the release-next branch to main and then triggers CI
# Usage: update-to-head.sh

set -e
REPO_NAME=$(basename $(git rev-parse --show-toplevel))

# Custom files
custom_files=$(cat <<EOT | tr '\n' ' '
openshift
EOT
)
openshift_files_msg=":open_file_folder: update OpenShift specific files"
robot_trigger_msg=":robot: triggering CI on branch 'release-next' after synching from upstream/main"

# Reset release-next to upstream/main.
git fetch upstream main
git checkout upstream/main -B release-next

# Update openshift's main and take all needed files from there.
git fetch openshift main
git checkout openshift/main $custom_files

# Apply midstream patches
if [[ -d openshift/patches ]]; then
  git apply openshift/patches/*
fi
# Apply midstream overrides
if [[ -d openshift/overrides ]]; then
  cp -r openshift/overrides/. .
  rm -rf openshift/overrides
fi
git add .

make generate/zz_filesystem_generated.go
git add $custom_files generate/zz_filesystem_generated.go
git commit -m "${openshift_files_msg}"

git push -f openshift release-next

# Trigger CI
git checkout release-next -B release-next-ci
date > ci
git add ci
git commit -m "${robot_trigger_msg}"
git push -f openshift release-next-ci

if hash hub 2>/dev/null; then
   # Test if there is already a sync PR in 
   COUNT=$(hub api -H "Accept: application/vnd.github.v3+json" repos/openshift-knative/${REPO_NAME}/pulls --flat \
    | grep -c "${robot_trigger_msg}") || true
   if [ "$COUNT" = "0" ]; then
      hub pull-request --no-edit -l "kind/sync-fork-to-upstream" -b openshift-knative/${REPO_NAME}:release-next -h openshift-knative/${REPO_NAME}:release-next-ci -m "${robot_trigger_msg}"
   fi
else
   echo "hub (https://github.com/github/hub) is not installed, so you'll need to create a PR manually."
fi
