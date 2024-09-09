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

openshift_pipelines
wait_pipelines_ready

echo Done
