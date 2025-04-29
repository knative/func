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

set -o errexit
set -o nounset
set -o pipefail

git_server() {
  echo "Creating Git Server Knative service..."
  cat << EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: func-git
  labels:
    app: git
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/max-scale: "1"
        autoscaling.knative.dev/min-scale: "1"
        client.knative.dev/user-image: quay.io/mvasek/gitserver
    spec:
      containers:
      - image: quay.io/mvasek/gitserver
        ports:
        - containerPort: 80
        resources: {}
status: {}
EOF

  kubectl wait ksvc --for=condition=RoutesReady --timeout=30s -l "app=git"
}

git_server

echo Done
