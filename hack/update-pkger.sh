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

source $(dirname $0)/../vendor/knative.dev/hack/library.sh

# Hack: touch a non-tagged file so that pkger doesn't complain
echo -e "package tools" > "$REPO_ROOT_DIR/hack/package.go"

go run ./vendor/github.com/markbates/pkger/cmd/pkger

# Hack: remove touched file.
rm "$REPO_ROOT_DIR/hack/package.go"

# Ensure pkged.go is migrated
gofmt -s -w pkged.go
