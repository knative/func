#!/usr/bin/env bash

# Copyright 2022 The OpenShift Knative Authors
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

# source of tasks (in this case to the project root folder)
source_path=$(dirname $(cd $(dirname $0) && pwd ))

openshift_pipelines() {
  echo "Installing Openshift Pipelines..."

  PIPELINE_OPERATOR_DEFAULT_CHANNEL=$(oc get packagemanifests openshift-pipelines-operator-rh -n openshift-marketplace -o json | jq '.status.defaultChannel' | tr -d '"')
  PIPELINE_OPERATOR_CHANNEL=${PIPELINE_OPERATOR_CHANNEL:-${PIPELINE_OPERATOR_DEFAULT_CHANNEL}}
  PIPELINE_TARGET_VERSION=$(oc get packagemanifests openshift-pipelines-operator-rh -n openshift-marketplace -o json | CHANNEL=$PIPELINE_OPERATOR_CHANNEL jq '.status.channels[] | select(.name==env.CHANNEL) | .currentCSV')

  echo Channel: $PIPELINE_OPERATOR_CHANNEL Target Version: $PIPELINE_TARGET_VERSION 

  cat << EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator-rh
  namespace: openshift-operators
spec:
  channel: $PIPELINE_OPERATOR_CHANNEL
  name: openshift-pipelines-operator-rh
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

}

wait_pipelines_ready() {
  echo "Waiting for Openshift Pipeline to get ready..."
  rc=1
  set +e
  for i in $(seq 5); do
    oc wait subscription.operators.coreos.com/openshift-pipelines-operator-rh -n openshift-operators --for=jsonpath='{.status.state}'="AtLatestKnown" --timeout=60s && \
        oc wait pod --for=condition=Ready --timeout=180s -n openshift-pipelines -l "app=tekton-pipelines-controller" && \
        oc wait pod --for=condition=Ready --timeout=180s -n openshift-pipelines -l "app=tekton-pipelines-webhook" && \
        rc=0 && break || (echo "Conditions are not matched. Retrying in 10 secs" && sleep 10)
  done
  set -e
  if (( $rc )); then
    echo "Installing Openshift pipelines has failed"
    exit 1
  fi
}

tekton_tasks() {
  echo "Creating Pipeline tasks..."
  oc apply -f ${source_path}/pkg/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml
  oc apply -f ${source_path}/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml
  oc apply -f ${source_path}/pkg/pipelines/resources/tekton/task/func-buildpacks/0.1/func-buildpacks.yaml
}

tasks_only=false
if [[ $# -ge 1 && "$1" == "--tasks-only" ]]; then
  tasks_only=true
elif [[ $# -ge 1 ]]; then
  echo "Unknown parameters, use '--tasks-only' to only install Tekton Tasks"
fi

if [ $tasks_only = false ] ; then
  openshift_pipelines
  wait_pipelines_ready
fi
tekton_tasks

echo Done
