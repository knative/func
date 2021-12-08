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

export TERM="${TERM:-dumb}"

main() {

  local kubernetes_version=v1.21.1
  local knative_version=v0.23.0
  local kourier_version=v0.23.0

  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Allocating...${me}"

  kubernetes
  serving
  dns
  eventing
  networking
  registry
  configure
  next_steps
  
  echo "${em}DONE${me}"
}

kubernetes() {
  echo "${em}① Kubernetes${me}"
  cat <<EOF | kind create cluster --wait=60s --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: kindest/node:${kubernetes_version}
    extraPortMappings:
    - containerPort: 30080
      hostPort: 80
      listenAddress: "127.0.0.1"
    - containerPort: 30443
      hostPort: 443
      listenAddress: "127.0.0.1"
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:50000"]
    endpoint = ["http://kind-registry:50000"]
EOF
  sleep 10
  kubectl wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=5m

}

serving() {
  echo "${em}② Knative Serving${me}"

  kubectl apply --filename https://github.com/knative/serving/releases/download/$knative_version/serving-crds.yaml

  sleep 5
  kubectl wait --for=condition=Established --all crd --timeout=5m

  curl -L -s https://github.com/knative/serving/releases/download/$knative_version/serving-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -


  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m

  kubectl get pod -A
}

dns() {
  echo "${em}③ DNS${me}"

  i=0; n=10
  while :; do
    kubectl patch configmap/config-domain \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"127.0.0.1.sslip.io":""}}' && break

    (( i+=1 ))
    if (( i>=n )); then
      echo "Unable to set knative domain"
      exit 1
    fi
    echo 'Retrying...'
    sleep 5
  done
}

networking() {
  echo "${em}④ Kourier Networking${me}"

  # Install Eourier
  kubectl apply --filename https://github.com/knative/net-kourier/releases/download/$kourier_version/kourier.yaml
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n kourier-system --timeout=5m
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m

  # Configure Knative to use Kourier
  kubectl patch configmap/config-network \
      --namespace knative-serving \
      --type merge \
      --patch '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'

  # Create NodePort ingress for kourier
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: kourier-ingress
  namespace: kourier-system
  labels:
    networking.knative.dev/ingress-provider: kourier
spec:
  type: NodePort
  selector:
    app: 3scale-kourier-gateway
  ports:
    - name: http2
      nodePort: 30080
      port: 80
      targetPort: 8080
    - name: https
      nodePort: 30443
      port: 443
      targetPort: 8443
EOF

  kubectl wait pod --for=condition=Ready -l '!job-name' -n kourier-system --timeout=5m
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m
}

eventing() {
  echo "${em}⑤ Knative Eventing${me}"

  # CRDs
  kubectl apply -f https://github.com/knative/eventing/releases/download/$knative_version/eventing-crds.yaml
  sleep 5
  kubectl wait --for=condition=Established --all crd --timeout=5m

  # Core
  curl -L -s https://github.com/knative/eventing/releases/download/$knative_version/eventing-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Channel
  curl -L -s https://github.com/knative/eventing/releases/download/$knative_version/in-memory-channel.yaml | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Broker
  curl -L -s https://github.com/knative/eventing/releases/download/$knative_version/mt-channel-broker.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

}

registry() {
  # see https://kind.sigs.k8s.io/docs/user/local-registry/

  echo "${em}⑥ Registry${me}"
  docker run -d --restart=always -p "50000:50000" --env REGISTRY_HTTP_ADDR="0.0.0.0:50000" --name "kind-registry" registry:2
  docker network connect "kind" "kind-registry"
  kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:50000"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

configure() {
  echo "${em}⑦ Configure Namespace${me}"

  # Create Namespace
  kubectl create namespace "func"

  # Default Broker
  kubectl apply -f - <<EOF
  apiVersion: eventing.knative.dev/v1
  kind: broker
  metadata:
   name: func-broker
   namespace: func
EOF

  # Default Channel
  kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: imc-channel
  namespace: knative-eventing
data:
  channelTemplateSpec: |
    apiVersion: messaging.knative.dev/v1
    kind: InMemoryChannel
EOF

  # Connect Default Broker->Channel
  kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-br-defaults
  namespace: knative-eventing
data:
  default-br-config: |
    # This is the cluster-wide default broker channel.
    clusterDefault:
      brokerClass: MTChannelBasedBroker
      apiVersion: v1
      kind: ConfigMap
      name: imc-channel
      namespace: knative-eventing
EOF

}

next_steps() {
  local red=$(tput bold)$(tput setaf 1)

  echo "${em}Configure Registry${me}"
  echo "If not in CI (running ci.sh): 
  echo "  ${red}add 'kind-registry' "to your local hosts${me} file:"
  echo "    echo \"127.0.0.1 kind-registry\" | sudo tee --append /etc/hosts"
  echo "  ${red}set registry as insecure${me} in the docker daemon config (/etc/docker/daemon.json on linux or ~/.docker/daemon.json on OSX):
  { \"insecure-registries\": [ \"kind-registry:50000\" ] }"
}

main "$@"
