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
source "$(go run knative.dev/hack/cmd/script presubmit-tests.sh)"

FUNC_REPO_BRANCH_REF="${PULL_PULL_SHA}"
export FUNC_REPO_BRANCH_REF

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
  install_node
  install_rust
  install_python
}

function install_node() {
  header "Installing Node.js"
  mkdir -p /tmp/nodejs
  wget https://nodejs.org/dist/${NODE_VERSION}/node-${NODE_VERSION}-${NODE_DISTRO}.tar.xz
  tar -xf node-${NODE_VERSION}-${NODE_DISTRO}.tar.xz -C /tmp/nodejs
  rm node-${NODE_VERSION}-${NODE_DISTRO}.tar.xz
  export PATH=/tmp/nodejs/node-${NODE_VERSION}-${NODE_DISTRO}/bin:$PATH
  subheader "Node.js version"
  node --version
  npm version
  npx --version
}

function install_rust() {
  header "Installing Rust"
  curl https://sh.rustup.rs -sSf > install.sh
  sh install.sh -y
  rm install.sh
  source "$HOME/.cargo/env"
  subheader "Rust version"
  cargo version
}

function install_python() {
  header "Installing Python"
  # Install Python if not already available
  command -v python >/dev/null 2>&1 || {
    apt-get update
    apt-get install -y python3 python3-pip python3-venv
    # Create symlink to ensure 'python' command works
    ln -sf /usr/bin/python3 /usr/bin/python
    ln -sf /usr/bin/pip3 /usr/bin/pip
  }
  # Ensure pip is up to date
  python -m pip install --upgrade pip
  subheader "Python version"
  python --version
  pip --version
}

function unit_tests() {
  local failed=0
  header "Checking embedded templates"
  go test knative.dev/func/pkg/filesystem -run '^\QTestFileSystems\E$/^\Qembedded\E$' -v || failed=1
  if (( failed )); then
     results_banner "Embedded templates check failed"
     exit "${failed}"
  fi
  header "Unit tests for $(go_mod_module_name)"
  make test || failed=1
  if (( failed )); then
    results_banner "Unit tests failed"
    exit "${failed}"
  fi
  template_tests
}

function template_tests() {
  header "Built-in template tests"
  make test-templates || failed=2
  if (( failed )); then
    results_banner "Built-in template tests failed"
    exit "${failed}"
  fi
}

function integration_tests() {
  local failed=0
  header "Skipping integration tests"
  # make test-integration || failed=1

  # if (( failed )); then
  #   results_banner "Integration tests failed"
  #   exit ${failed}
  # fi
}

main "$@"
