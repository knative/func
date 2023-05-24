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

set -o errexit
set -o nounset
set -o pipefail

export TERM="${TERM:-dumb}"

tekton_release="previous/v0.47.0"
git_clone_release="0.4"
namespace="${NAMESPACE:-default}"
tasks_source_path="$(dirname "$(cd "$(dirname "$0")" && pwd )")"

tekton() {
  echo "Installing Tekton..."
  kubectl apply -f "https://storage.googleapis.com/tekton-releases/pipeline/${tekton_release}/release.yaml"
  sleep 10
  kubectl wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-controller"
  kubectl wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-webhook"
  sleep 10

  kubectl create clusterrolebinding "${namespace}:knative-serving-namespaced-admin" \
  --clusterrole=knative-serving-namespaced-admin  --serviceaccount="${namespace}:default"
}

tekton_tasks() {
  echo "Creating Pipeline tasks..."
  kubectl apply -f "https://raw.githubusercontent.com/tektoncd/catalog/master/task/git-clone/${git_clone_release}/git-clone.yaml"
  kubectl apply -f "${tasks_source_path}/pkg/pipelines/resources/tekton/task/func-buildpacks/0.1/func-buildpacks.yaml"
  kubectl apply -f "${tasks_source_path}/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml"
  kubectl apply -f "${tasks_source_path}/pkg/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml"
}

## Parse input parameters
# Supported parameters:
# --tasks-only - install only Tekton Tasks
tasks_only=false
if [ $# -gt 1 ] ; then
  echo "Unknown parameters, use '--tasks-only' to only install Tekton Tasks"
  exit 1
fi
if [ $# -eq 1 ] ; then
    if [ "$1" == "--tasks-only" ]
      then
        tasks_only=true
    else
      echo "Unknown parameter '${1}', use '--tasks-only' to only install Tekton Tasks"
      exit 1
    fi
fi

## Installation phase
if [ $tasks_only = false ] ; then
  tekton
fi
tekton_tasks

echo Done
