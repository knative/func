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

# Usage: openshift/release/mirror-upstream-branches.sh
# This should be run from the basedir of the repo with no arguments


set -exo pipefail

TMPDIR=$(mktemp -d knativeFuncBranchingCheckXXXX -p /tmp/)
readonly TMPDIR

git fetch upstream --tags --force # use force to not complain about existing tags with same name in origin
git fetch openshift

# Ignore release 1.7-1.14 and only sync starting from 1.15
cat >> "$TMPDIR"/midstream_branches <<EOF
1.7
1.8
1.9
1.10
1.11
1.12
1.13
1.14
1.15
EOF

git branch --list -a "upstream/release-1.*" | cut -f3 -d'/' | cut -f2 -d'-' > "$TMPDIR"/upstream_branches
git branch --list -a "openshift/release-v1.*" | cut -f3 -d'/' | cut -f2 -d'v' | cut -f1,2 -d'.' >> "$TMPDIR"/midstream_branches

sort -o "$TMPDIR"/midstream_branches "$TMPDIR"/midstream_branches
sort -o "$TMPDIR"/upstream_branches "$TMPDIR"/upstream_branches
comm -32 "$TMPDIR"/upstream_branches "$TMPDIR"/midstream_branches > "$TMPDIR"/new_branches

if [ ! -s "$TMPDIR/new_branches" ]; then
    echo "no new branch, exiting"
    exit 0
fi

while read -r UPSTREAM_BRANCH; do
  echo "found upstream branch: $UPSTREAM_BRANCH"
  readonly UPSTREAM_TAG="knative-v$UPSTREAM_BRANCH.0"
  readonly MIDSTREAM_BRANCH="release-v$UPSTREAM_BRANCH"

  openshift/release/create-release-branch.sh "$UPSTREAM_TAG" "$MIDSTREAM_BRANCH"

  # we could check the error code, but we 'set -e', so assume we're fine
  git push openshift "$MIDSTREAM_BRANCH"
done < "$TMPDIR/new_branches"
