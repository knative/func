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
# Installs the Pipelines-as-code controller
#

source "$(dirname "$(realpath "$0")")/common.sh"

function install_pac() {
    echo "${blue}Installing the Pipelines-as-Code Controller${reset}"

    local -r pac_ctr_host="${PAC_CONTROLLER_HOSTNAME:-pac-ctr.127.0.0.1.sslip.io}"
    local -r pac_version="v0.17.1"

    # Install Pipelines as Code
    $KUBECTL apply -f "https://raw.githubusercontent.com/openshift-pipelines/pipelines-as-code/release-${pac_version}/release.k8s.yaml"
    sleep 5
    $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n pipelines-as-code --timeout=5m

    # Install ingress for the PaC controller. This is used by VCS Webhooks.
    $KUBECTL apply -f - << EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: pipelines-as-code
  namespace: pipelines-as-code
spec:
  ingressClassName: contour-external
  rules:
  - host: ${pac_ctr_host}
    http:
      paths:
      - backend:
          service:
            name: pipelines-as-code-controller
            port:
              number: 8080
        pathType: Prefix
        path: /
EOF
  echo "the Pipeline as Code controller is available at: http://${pac_ctr_host}"
  echo "${green}✅ PAC${reset}"
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
      install_pac
  }
  main "$@"
fi
