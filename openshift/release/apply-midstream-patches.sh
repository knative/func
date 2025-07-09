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

# Applies the midstream patches and runs update-codegen.sh

# Apply midstream patches
if [[ -d openshift/patches ]]; then
  git apply openshift/patches/*
fi

# Apply midstream overrides
if [[ -d openshift/overrides ]]; then
  cp -r openshift/overrides/. .
  rm -rf openshift/overrides
fi

# Apply midstream patches using scripts
if [[ -d openshift/scripts ]]; then
  for script in openshift/scripts/*.sh; do
    "$script"
  done
fi

./hack/update-codegen.sh

make generate/zz_filesystem_generated.go
