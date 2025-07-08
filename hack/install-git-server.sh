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
  echo "Creating Git Server"

  local name="func-git"

  local namespace
  namespace="$(kubectl config view --minify --output 'jsonpath={..namespace}')"
  namespace="${namespace:-"default"}"

  local -r func_git_host="${name}.${namespace}.localtest.me"

  kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: "${name}"
  namespace: "${namespace}"
  labels:
    app.kubernetes.io/name: "${name}"
spec:
  containers:
    - name: "${name}"
      image: quay.io/mvasek/gitserver
      ports:
        - containerPort: 8080
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: "${name}"
  namespace: "${namespace}"
spec:
  selector:
    app.kubernetes.io/name: "${name}"
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: "${name}"
  namespace: "${namespace}"
spec:
  ingressClassName: contour-external
  rules:
    - host: "${func_git_host}"
      http:
        paths:
          - backend:
              service:
                name: "${name}"
                port:
                  number: 80
            pathType: Prefix
            path: /
EOF

  echo "starting func-git service at: ${func_git_host}"

  kubectl wait --for=condition=Ready "pod/${name}" --timeout=30s
}

git_server

echo Done
