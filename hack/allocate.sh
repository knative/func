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

  local kubernetes_version=v1.24.6
  local knative_serving_version=v1.8.0
  local knative_eventing_version=v1.8.0
  local contour_version=v1.8.0

  # shellcheck disable=SC2155
  local em=$(tput bold)$(tput setaf 2)
  # shellcheck disable=SC2155
  local me=$(tput sgr0)

  echo "${em}Allocating...${me}"

  kubernetes
  serving
  dns
  eventing
  networking
  registry
  namespace
  dapr_runtime
  next_steps
  
  echo "${em}DONE${me}"
}

kubernetes() {
  echo "${em}① Kubernetes${me}"
  cat <<EOF | kind create cluster --name=func --wait=60s --config=-
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
    endpoint = ["http://func-registry:5000"]
EOF
  sleep 10
  kubectl wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=5m

}

serving() {
  echo "${em}② Knative Serving${me}"

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

networking() {
  echo "${em}④ Contour Ingress${me}"

  # Install a properly configured Contour.
  kubectl apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/contour.yaml"
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=5m

  # Install the Knative Contour controller.
  kubectl apply -f "https://github.com/knative/net-contour/releases/download/knative-${contour_version}/net-contour.yaml"
  sleep 5
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m

  # Configure Knative Serving to use Contour.
  kubectl patch configmap/config-network \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"ingress-class":"contour.ingress.networking.knative.dev"}}'

  # Patch type from LoadBalancer to NodePort and fix nodePorts for http to 30080 and https to 30443
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: envoy
  namespace: contour-external
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp
  labels:
    networking.knative.dev/ingress-provider: contour
spec:
  type: NodePort
  selector:
    app: envoy
  externalTrafficPolicy: Local
  ports:
    - name: http
      nodePort: 30080
      port: 80
      targetPort: 8080
    - name: https
      nodePort: 30443
      port: 443
      targetPort: 8443
EOF

  kubectl wait pod --for=condition=Ready -l '!job-name' -n contour-external --timeout=5m
  kubectl wait pod --for=condition=Ready -l '!job-name' -n knative-serving --timeout=5m
}

eventing() {
  echo "${em}⑤ Knative Eventing${me}"

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
  docker run -d --restart=always -p "127.0.0.1:50000:5000" --name "func-registry" registry:2
  docker network connect "kind" "func-registry"
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

  # Install Dapr Runtime
  dapr init --kubernetes --wait

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
