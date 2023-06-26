#!/usr/bin/env bash

# Copyright 2023 The Knative Authors
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

export DISABLE_MD_LINTING=1
export DISABLE_MD_LINK_CHECK=1
export PRESUBMIT_TEST_FAIL_FAST=1
export NODE_VERSION=v18.10.0
export NODE_DISTRO=linux-x64

export KNATIVE_SERVING_VERSION=${KNATIVE_SERVING_VERSION:-latest}
export KNATIVE_EVENTING_VERSION=${KNATIVE_EVENTING_VERSION:-latest}
source $(dirname $0)/../vendor/knative.dev/hack/presubmit-tests.sh

function post_build_tests() {
  local failed=0
  header "Ensuring code builds cross-platform"
  make cross-platform || failed=1
  if (( failed )); then
    results_banner "Build failed"
    exit ${failed}
  fi
}

function pre_unit_tests() {
  subheader "Installing Node.js"
  mkdir -p /tmp/nodejs
  wget https://nodejs.org/dist/${NODE_VERSION}/node-${NODE_VERSION}-${NODE_DISTRO}.tar.xz
  tar -xJvf node-${NODE_VERSION}-${NODE_DISTRO}.tar.xz -C /tmp/nodejs
  export PATH=/tmp/nodejs/node-${NODE_VERSION}-${NODE_DISTRO}/bin:$PATH
  node --version
  npm version
  npx --version
}

function unit_tests() {
  local failed=0
  header "Unit tests for $(go_mod_module_name)"
  make test || failed=1
  if (( failed )); then
    results_banner "Unit tests failed"
    exit ${failed}
  fi

  header "Running built-in template tests"
  make test-templates || failed=2
  if (( failed )); then
    results_banner "Template tests failed"
    exit ${failed}
  fi
}

function integration_tests() {
  local failed=0
  header "Running integration tests"
  make test-integration || failed=1

  if (( failed )); then
    results_banner "Integration tests failed"
    exit ${failed}
  fi
}

main "$@"
