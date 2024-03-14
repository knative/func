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

CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
export TERM="${TERM:-dumb}"

main() {

  local knative_serving_version="v$(get_latest_release_version "knative" "serving")"
  local knative_eventing_version="v$(get_latest_release_version "knative" "eventing")"
  local contour_version="v$(get_latest_release_version "knative-extensions" "net-contour")"

  # Kubernetes Version node image per Kind releases (full hash is suggested):
  # https://github.com/kubernetes-sigs/kind/releases
  local kind_node_version=v1.29.2@sha256:51a1434a5397193442f0be2a297b488b6c919ce8a3931be0ce822606ea5ca245

  # shellcheck disable=SC2155
  local em=$(tput bold)$(tput setaf 2)
  # shellcheck disable=SC2155
  local me=$(tput sgr0)

  echo "${em}Allocating...${me}"

  kubernetes
  loadbalancer
  ( set -o pipefail; (serving && dns && networking) 2>&1 | sed  -e 's/^/svr /')&
  ( set -o pipefail; (eventing && namespace) 2>&1 | sed  -e 's/^/evt /')&
  ( set -o pipefail; registry 2>&1 | sed  -e 's/^/reg /') &
  ( set -o pipefail; dapr_runtime 2>&1 | sed  -e 's/^/dpr /')&

  local job
  for job in $(jobs -p); do
    wait "$job"
  done

  next_steps
  
  echo "${em}DONE${me}"
}

# Returns the current branch.
# Taken from knative/hack. The function covers Knative CI use cases and local variant.
function current_branch() {
  local branch_name=""
  # Get the branch name from Prow's env var, see https://github.com/kubernetes/test-infra/blob/master/prow/jobs.md.
  # Otherwise, try getting the current branch from git.
  (( IS_PROW )) && branch_name="${PULL_BASE_REF:-}"
  [[ -z "${branch_name}" ]] && branch_name="${GITHUB_BASE_REF:-}"
  [[ -z "${branch_name}" ]] && branch_name="$(git rev-parse --abbrev-ref HEAD)"
  echo "${branch_name}"
}

# Returns whether the current branch is a release branch.
function is_release_branch() {
  [[ $(current_branch) =~ ^release-[0-9\.]+$ ]]
}

# Retrieve latest version from given Knative repository tags
# On 'main' branch the latest released version is returned
# On 'release-x.y' branch the latest patch version for 'x.y.*' is returned
# Similar to hack/library.sh get_latest_knative_yaml_source()
function get_latest_release_version() {
    local org_name="$1"
    local repo_name="$2"
    local major_minor=""
    if is_release_branch; then
      local branch_name
      branch_name="$(current_branch)"
      major_minor="${branch_name##release-}"
    fi
    local version
    version="$(git ls-remote --tags --ref https://github.com/"${org_name}"/"${repo_name}".git \
      | grep "${major_minor}" \
      | cut -d '-' -f2 \
      | cut -d 'v' -f2 \
      | sort -Vr \
      | head -n 1)"
    echo "${version}"
}

kubernetes() {
  echo "${em}① Kubernetes${me}"
  cat <<EOF | kind create cluster --name=func --wait=60s --config=-
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
  kubectl wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=5m

}

serving() {
  echo "${em}② Knative Serving${me}"
  echo "Version: ${knative_serving_version}"

  kubectl apply --filename https://github.com/knative/serving/releases/download/knative-$knative_serving_version/serving-crds.yaml

  sleep 5
  kubectl wait --for=condition=Established --all crd --timeout=5m

  curl -L -s https://github.com/knative/serving/releases/download/knative-$knative_serving_version/serving-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | yq | kubectl apply -f -


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

loadbalancer() {
  echo "Install load balancer."
  kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml"
  sleep 5
  kubectl wait --namespace metallb-system \
    --for=condition=ready pod \
    --selector=app=metallb \
    --timeout=300s

  local kind_addr
  kind_addr="$($CONTAINER_ENGINE container inspect func-control-plane | jq '.[0].NetworkSettings.Networks.kind.IPAddress' -r)"

  echo "Setting up address pool."
  kubectl apply -f - <<EOF
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
}

networking() {
  echo "${em}④ Contour Ingress${me}"
  echo "Version: ${contour_version}"

  echo "Install a properly configured Contour."
  kubectl apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/contour.yaml"
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=10m

  echo "Install the Knative Contour controller."
  kubectl apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/net-contour.yaml"
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=10m

  echo "Configure Knative Serving to use Contour."
  kubectl patch configmap/config-network \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"ingress-class":"contour.ingress.networking.knative.dev"}}'

  kubectl wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=10m
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=10m
}

eventing() {
  echo "${em}⑤ Knative Eventing${me}"
  echo "Version: ${knative_eventing_version}"

  # CRDs
  kubectl apply -f https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/eventing-crds.yaml
  sleep 5
  kubectl wait --for=condition=Established --all crd --timeout=5m

  # Core
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/eventing-core.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | yq | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Channel
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/in-memory-channel.yaml | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

  # Broker
  curl -L -s https://github.com/knative/eventing/releases/download/knative-$knative_eventing_version/mt-channel-broker.yaml | yq 'del(.spec.template.spec.containers[]?.resources)' -y | yq 'del(.metadata.annotations."knative.dev/example-checksum")' -y | yq | kubectl apply -f -
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-eventing --timeout=5m

}

registry() {
  # see https://kind.sigs.k8s.io/docs/user/local-registry/

  echo "${em}⑥ Registry${me}"
  if [ "$CONTAINER_ENGINE" == "docker" ]; then
    $CONTAINER_ENGINE run -d --restart=always -p "127.0.0.1:50000:5000" --name "func-registry" registry:2
    $CONTAINER_ENGINE network connect "kind" "func-registry"
  elif [ "$CONTAINER_ENGINE" == "podman" ]; then
    $CONTAINER_ENGINE run -d --restart=always -p "127.0.0.1:50000:5000" --net=kind --name "func-registry" registry:2
  fi

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

  # Make the registry available in cluster under registry.default.svc.cluster.local:5000.
  # This is useful since for "*.local" registries HTTP (not HTTPS) is used by default by some applications.
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: registry
  namespace: default
spec:
  type: ExternalName
  externalName: func-registry
EOF
}

namespace() {
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

dapr_runtime() {
  echo "${em}⑦ Dapr${me}"
  echo "Version:\\n$(dapr version)"

  local dapr_flags=""
  if [ "${GITHUB_ACTIONS:-false}" = "true" ]; then
    dapr_flags="--image-registry=ghcr.io/dapr --log-as-json"
  fi

  # Install Dapr Runtime
  # shellcheck disable=SC2086
  dapr init ${dapr_flags} --kubernetes --wait

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
  echo "${em}- Redis ${me}"
  helm repo add bitnami https://charts.bitnami.com/bitnami
  helm install redis bitnami/redis --set image.tag=6.2
  helm repo update

  # 2) Expose a Redis-backed Dapr State Storage component
  echo "${em}- State Storage Component${me}"
  kubectl apply -f - << EOF
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
  echo "${em}- Pub/Sub Component${me}"
  kubectl apply -f - << EOF
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

}


next_steps() {
  # shellcheck disable=SC2155
  local red=$(tput bold)$(tput setaf 1)

  echo "${em}Image Registry${me}"
  echo "If not in CI (running ci.sh): "
  echo "  ${red}set registry as insecure${me} in the docker daemon config (/etc/docker/daemon.json on linux or ~/.docker/daemon.json on OSX):"
  echo "    { \"insecure-registries\": [ \"localhost:50000\" ] }"
}

main "$@"
