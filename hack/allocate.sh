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
# Allocate a Kind cluster with Knative, Kourier and a local container registry.
#

set -o errexit
set -o nounset
set -o pipefail

main() {

  local serving_version=v0.24.0
  local eventing_version=v0.24.0
  local kourier_version=v0.24.0

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Allocating...${me}"

  cluster
  serving
  eventing
  networking
  registry
  next_steps
  
  echo "${em}DONE${me}"
}

cluster() {
  echo "${em}① Cluster${me}"
  cat <<EOF | kind create cluster --wait=60s --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
    extraPortMappings:
    - containerPort: 30080
      hostPort: 80
      listenAddress: "127.0.0.1"
    - containerPort: 30443
      hostPort: 443
      listenAddress: "127.0.0.1"
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5000"]
    endpoint = ["http://kind-registry:5000"]
EOF
}

serving() {
  echo "${em}② Knative Serving${me}"
  kubectl apply --filename https://github.com/knative/serving/releases/download/$serving_version/serving-crds.yaml
  sleep 5
  curl -L -s https://github.com/knative/serving/releases/download/$serving_version/serving-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  echo "Resources being initialized"
  sleep 10
  kubectl get pod -n knative-serving
}

eventing() {
  echo "${em}③ Knative Eventing${me}"
  # CRDs
  kubectl apply --filename https://github.com/knative/eventing/releases/download/$eventing_version/eventing-crds.yaml
  sleep 5
  # Core
  curl -L -s https://github.com/knative/eventing/releases/download/$eventing_version/eventing-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  # Channel
  # yq fails parsing in-memory-channel.yaml due to duplicate declaration of the
  # &everything anchor.  Upon investigation, the yq statements may actually not be necessary.
  # curl -L -s https://github.com/knative/eventing/releases/download/$eventing_version/in-memory-channel.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  curl -L -s https://github.com/knative/eventing/releases/download/$eventing_version/in-memory-channel.yaml | kubectl apply -f -
  # Broker
  curl -L -s https://github.com/knative/eventing/releases/download/$eventing_version/mt-channel-broker.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  # Echo
  echo "Resources being initialized"
  sleep 5
  kubectl get pod -n knative-eventing
}

networking() {
  echo "${em}④ Kourier Networking${me}"
  kubectl apply --filename https://github.com/knative/net-kourier/releases/download/$kourier_version/kourier.yaml
  kubectl patch configmap/config-network \
      --namespace knative-serving \
      --type merge \
      --patch '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
  echo "Resources being initialized"
  sleep 5
  kubectl get pod -n kourier-system
}

registry() {
  # see https://kind.sigs.k8s.io/docs/user/local-registry/

  echo "${em}⑤ Container Registry${me}"
  docker run -d --restart=always -p "5000:5000" --name "kind-registry" registry:2
  docker network connect "kind" "kind-registry"
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:5000"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

next_steps() {
  local red=$(tput bold)$(tput setaf 1)

  echo "${em}⑥ Configure Registry${me}"
  echo "If not in CI (running ci.sh): 
  echo "  ${red}add 'kind-registry' "to your local hosts${me} file:"
  echo "    echo \"127.0.0.1 kind-registry\" | sudo tee --append /etc/hosts"
  echo "  ${red}set registry as insecure${me} in the docker daemon config (/etc/docker/daemon.json on linux or ~/.docker/daemon.json on OSX):
  { \"insecure-registries\": [ \"kind-registry:5000\" ] }"
}

main "$@"
