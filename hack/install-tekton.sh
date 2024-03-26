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
# Install Tekton and required tasks in the cluster
#

source "$(dirname "$(realpath "$0")")/common.sh"

install_tekton() {
  echo "${blue}Installing Tekton${reset}"

  tekton_release="previous/v0.49.0"
  namespace="${NAMESPACE:-default}"

  $KUBECTL apply -f "https://storage.googleapis.com/tekton-releases/pipeline/${tekton_release}/release.yaml"
  sleep 10
  $KUBECTL wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-controller"
  $KUBECTL wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-webhook"
  sleep 10

  $KUBECTL create clusterrolebinding "${namespace}:knative-serving-namespaced-admin" --clusterrole=knative-serving-namespaced-admin --serviceaccount="${namespace}:default"

  echo "${green}✅ Tekton${reset}"
}

# Invoke only when run directly
# Be a library when sourced
if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    install_tekton
  }
  main "$@"
fi
