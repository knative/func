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
# Configures the current cluster for use with Functions
# Sets up the namespace, networking, configures ingress to be a nodeport and 
# sets up the default domain.
#

set -o errexit
set -o nounset
set -o pipefail

DEFAULT_NAMESPACE=func

show_help() {
  cat << EOF
  Configure a local cluster for use with Functions.

  Usage: $(basename "$0") <options>

    -h, --help                              Display help
    -n, --namespace                         The namespace to use for Functions (default: $DEFAULT_NAMESPACE)
EOF

}

main() {
  local namespace="$DEFAULT_NAMESPACE"

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  parse_command_line "$@"

  echo "${em}Configuring...${me}"

  namespace 
  network
  kourier_nodeport
  default_domain

  sleep 5
  kubectl --namespace kourier-system get service kourier

  echo "${em}DONE${me}"
}

parse_command_line() {
  while :; do
    case "${1:-}" in
      -h|--help)
        show_help
        exit
        ;;
      -n|--namespace)
        if [[ -n "${2:-}" ]]; then
          namespace="$2"
          shift
        else
          echo "ERROR: '-n|--namespace' cannot be empty." >&2
          show_help
          exit 1
        fi
        ;;
      *)
        break
        ;;
    esac
  done
}

namespace() {
  echo "${em}① Namespace${me}"
  kubectl create namespace "$namespace"
  kubectl label namespace "$namespace" knative-eventing-injection=enabled
}

network() {
  echo "${em}② Network${me}"
  echo "Registering Kourier as ingress"
  echo "Enabling subdomains"
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-network
  namespace: knative-serving
data:
  # Use Kourier for the networking layer
  ingress.class: kourier.ingress.networking.knative.dev

  # If there exists an annotation 'func.subdomain' on the service, use it 
  # instead of the default name.namespace
  domainTemplate: |-
    {{if index .Annotations "func.subdomain" -}}
      {{- index .Annotations "func.subdomain" -}}
    {{else -}}
      {{- .Name}}.{{.Namespace -}}
    {{end -}}
    .{{.Domain}}

EOF
}

kourier_nodeport() {
  echo "${em}③ Nodeport${me}"
  echo 'Setting Kourier service to type NodePort'
  # Patch for changing kourier to a NodePort for installations where a 
  # LoadBalancer is not available (for example local Kind clusters)
  # kubectl patch -n kourier-system services/kourier -p "$(cat configure-kourier-nodeport.yaml)"
  kubectl patch services/kourier \
    --namespace kourier-system \
    --type merge \
    --patch '{
  "spec": {
    "ports": [
      {
        "name": "http2",
        "nodePort": 30080,
        "port": 80,
        "protocol": "TCP",
        "targetPort": 8080
      },
      {
        "name": "https",
        "nodePort": 30443,
        "port": 443,
        "protocol": "TCP",
        "targetPort": 8443
      }
    ],
    "type": "NodePort"
  }
}'
}

default_domain() {
  echo "${em}④ Default Domains${me}"
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-domain
  namespace: knative-serving
data:
  example.com: |
    selector:
      func.domain: "example.com"
  example.org: |
    selector:
      func.domain: "example.org"
  # Default is local only.
  cluster.local: ""
EOF
}

main "$@"
