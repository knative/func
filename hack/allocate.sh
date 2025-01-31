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

source "$(dirname "$(realpath "$0")")/common.sh"
# this is where versions of common components are (like knative)
source "$(dirname "$(realpath "$0")")/component-versions.sh"

main() {
  echo "${blue}Allocating${reset}"

  set_versions
  kubernetes
  loadbalancer

  echo "${blue}Beginning Cluster Configuration${reset}"
  echo "Tasks will be executed in parallel.  Logs will be prefixed:"
  echo "svr:  Serving, DNS and Networking"
  echo "evt:  Eventing and Namespace"
  echo "reg:  Local Registry"
  echo "dpr:  Dapr Runtime"
  echo ""

  ( set -o pipefail; (serving && dns && networking) 2>&1 | sed  -e 's/^/svr /')&
  ( set -o pipefail; (eventing && namespace) 2>&1 | sed  -e 's/^/evt /')&
  ( set -o pipefail; registry 2>&1 | sed  -e 's/^/reg /') &
  ( set -o pipefail; dapr_runtime 2>&1 | sed  -e 's/^/dpr /')&

  local job
  for job in $(jobs -p); do
    wait "$job"
  done

  next_steps

  echo -e "\n${green}🎉 DONE${reset}\n"
}

kubernetes() {
  cat <<EOF | $KIND create cluster --name=func --kubeconfig="${KUBECONFIG}" --wait=60s --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: kindest/node:${kind_node_version}
    extraPortMappings:
    - containerPort: 80
      hostPort: 80
      listenAddress: "127.0.0.1"
    - containerPort: 433
      hostPort: 443
      listenAddress: "127.0.0.1"
    - containerPort: 30022
      hostPort: 30022
      listenAddress: "127.0.0.1"
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:50000"]
    endpoint = ["http://func-registry:5000"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.default.svc.cluster.local:5000"]
    endpoint = ["http://func-registry:5000"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."ghcr.io"]
    endpoint = ["http://func-registry:5000"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."quay.io"]
    endpoint = ["http://func-registry:5000"]
EOF
  sleep 10
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=5m
  echo "${green}✅ Kubernetes${reset}"
}

serving() {
  echo "${blue}Installing Serving${reset}"
  echo "Version: ${knative_serving_version}"

  $KUBECTL apply --filename https://github.com/knative/serving/releases/download/knative-$knative_serving_version/serving-crds.yaml

  sleep 2
  $KUBECTL wait --for=condition=Established --all crd --timeout=5m

  curl -L -s https://github.com/knative/serving/releases/download/knative-$knative_serving_version/serving-core.yaml | $KUBECTL apply -f -

  sleep 2
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m

  $KUBECTL get pod -A
  echo "${green}✅ Knative Serving${reset}"
}

dns() {
  echo "${blue}Configuring DNS${reset}"

  i=0; n=10
  while :; do
    $KUBECTL patch configmap/config-domain \
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
  echo "${green}✅ DNS${reset}"
}

loadbalancer() {
  echo "${blue}Installing Load Balancer (Metallb)${reset}"
  $KUBECTL apply -f "https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml"
  sleep 5
  $KUBECTL wait --namespace metallb-system \
    --for=condition=ready pod \
    --selector=app=metallb \
    --timeout=300s

  local kind_addr
  kind_addr="$($CONTAINER_ENGINE container inspect func-control-plane | jq '.[0].NetworkSettings.Networks.kind.IPAddress' -r)"

  echo "Setting up address pool."
  $KUBECTL apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: example
  namespace: metallb-system
spec:
  addresses:
  - ${kind_addr}-${kind_addr}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
EOF
  echo "${green}✅ Loadbalancer${reset}"
}

networking() {
  echo "${blue}Installing Ingress Controller (Contour)${reset}"
  echo "Version: ${contour_version}"

  echo "Installing a configured Contour."
  $KUBECTL apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/contour.yaml"
  sleep 5
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=10m

  echo "Installing the Knative Contour controller."
  $KUBECTL apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/net-contour.yaml"
  sleep 5
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=10m

  echo "Configuring Knative Serving to use Contour."
  $KUBECTL patch configmap/config-network \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"ingress-class":"contour.ingress.networking.knative.dev"}}'

  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=10m
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=10m
  echo "${green}✅ Ingress${reset}"
}

eventing() {
  echo "${blue}Installing Eventing${reset}"
  echo "Version: ${knative_eventing_version}"

  # CRDs
  $KUBECTL apply -f https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/eventing-crds.yaml
  sleep 5
  $KUBECTL wait --for=condition=Established --all crd --timeout=5m

  # Core
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/eventing-core.yaml | $KUBECTL apply -f -
  sleep 5
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Channel
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/in-memory-channel.yaml | $KUBECTL apply -f -
  sleep 5
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Broker
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/mt-channel-broker.yaml | $KUBECTL apply -f -
  sleep 5
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  echo "${green}✅ Eventing${reset}"
}

registry() {
  # see https://kind.sigs.k8s.io/docs/user/local-registry/

  echo "${blue}Creating Registry${reset}"
  if [ "$CONTAINER_ENGINE" == "docker" ]; then
    $CONTAINER_ENGINE run -d --restart=always -p "127.0.0.1:50000:5000" --name "func-registry" registry:2
    $CONTAINER_ENGINE network connect "kind" "func-registry"
  elif [ "$CONTAINER_ENGINE" == "podman" ]; then
    $CONTAINER_ENGINE run -d --restart=always -p "127.0.0.1:50000:5000" --net=kind --name "func-registry" registry:2
  fi

  $KUBECTL apply -f - <<EOF
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

  # Make the registry available in cluster under registry.default.svc.cluster.local:5000.
  # This is useful since for "*.local" registries HTTP (not HTTPS) is used by default by some applications.
  $KUBECTL apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: registry
  namespace: default
spec:
  type: ExternalName
  externalName: func-registry
EOF
  echo "${green}✅ Registry${reset}"
}

namespace() {
  echo "${blue}Configuring Namespace \"func\"${reset}"

  # Create Namespace
  $KUBECTL create namespace "func"

  # Default Broker
  $KUBECTL apply -f - <<EOF
  apiVersion: eventing.knative.dev/v1
  kind: Broker
  metadata:
   name: func-broker
   namespace: func
EOF

  # Default Channel
  $KUBECTL apply -f - << EOF
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
  $KUBECTL apply -f - << EOF
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

  echo "${green}✅ Namespace${reset}"
}

dapr_runtime() {
  echo "${blue}Installing Dapr Runtime${reset}"
  echo "Version:\\n$($DAPR version)"

  local dapr_flags=""
  if [ "${GITHUB_ACTIONS:-false}" = "true" ]; then
    dapr_flags="--image-registry=ghcr.io/dapr --log-as-json"
  fi

  # Install Dapr Runtime
  # shellcheck disable=SC2086
  $DAPR init ${dapr_flags} --kubernetes --wait

  # Enalble Redis Persistence and Pub/Sub
  #
  # 1) Redis
  # Creates a Redis leader with three replicas
  # TODO: helm and the bitnami charts are likely not necessary.  The Bitnami
  # charts do tweak quite a few settings, but I am skeptical it is necessary
  # in a CI/CD environment, as it does add nontrivial support overhead.
  # TODO: If the bitnami redis chart seems worth the effort, munge this command
  # to only start a single instance rather than four.
  # helm repo add bitnami https://charts.bitnami.com/bitnami
  echo "${blue}- Redis ${reset}"
  $HELM repo add bitnami https://charts.bitnami.com/bitnami
  $HELM install redis bitnami/redis --set image.tag=6.2
  $HELM repo update

  # 2) Expose a Redis-backed Dapr State Storage component
  echo "${blue}- State Storage Component${reset}"
  $KUBECTL apply -f - << EOF
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: statestore
  namespace: default
spec:
  type: state.redis
  version: v1
  metadata:
  - name: redisHost
    value: redis-master.default.svc.cluster.local:6379
  - name: redisPassword
    secretKeyRef:
      name: redis
      key: redis-password
EOF

  # 3) Expose A Redis-backed Dapr Pub/Sub Component
  echo "${blue}- Pub/Sub Component${reset}"
  $KUBECTL apply -f - << EOF
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: pubsub
  namespace: default
spec:
  type: pubsub.redis
  version: v1
  metadata:
  - name: redisHost
    value: redis-master.default.svc.cluster.local:6379
  - name: redisPassword
    secretKeyRef:
      name: redis
      key: redis-password
EOF

  echo "${green}✅ Dapr Runtime${reset}"
}

next_steps() {
  echo -e ""
  echo -e "${blue}Next Steps${reset}"
  echo -e "${blue}----------${reset}"
  echo -e ""
  echo -e "${grey}REGISTRY"
  echo -e "Before using the cluster for integration and E2E tests, please run \"${reset}registry.sh${grey}\" (Linux systems) which will configure podman or docker to communicate with the standalone container registry without TLS."
  echo -e ""
  echo -e "For other operating systems, or to do this manually, edit the docker daemon config (/etc/docker/daemon.json on linux and ~/.docker/daemon.json on OSX), add:"
  echo -e "${reset}{ \"insecure-registries\": [ \"localhost:50000\" ] }"
  echo -e ""
  echo -e "${grey}For podman, edit /etc/container/registries.conf to include:"
  echo -e "${reset}[[registry-insecure-local]]\nlocation = \"localhost:50000\"\ninsecure = true\n"
  echo -e "${grey}The cluster and resources can be removed with \"${reset}delete.sh\""
  echo -e ""
  echo -e "${grey}KUBECONFIG"
  echo -e "The kubeconfig for your test cluster has been saved to:${reset}"
  echo -e "${KUBECONFIG}"
  echo -e ""
}

main "$@"
