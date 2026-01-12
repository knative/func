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

source "$(cd "$(dirname "$0")" && pwd)/common.sh"
# this is where versions of common components are (like knative)
source "$(cd "$(dirname "$0")" && pwd)/component-versions.sh"

main() {
  local max_attempts="${FUNC_CLUSTER_RETRIES:-1}"
  local attempt=0

  if [ $max_attempts -gt 1 ]; then
    echo "${blue}Cluster allocation will retry up to ${max_attempts} time(s)${reset}"
  fi

  until [ $attempt -ge $max_attempts ]
  do
    attempt=$((attempt+1))

    if [ $max_attempts -gt 1 ]; then
      echo "${blue}------------------ Attempt $attempt ------------------${reset}"
    fi

    if allocate_cluster; then
      if [ $max_attempts -gt 1 ]; then
        echo "${green}------------------ Success! Cluster allocated ------------------${reset}"
      fi
      return 0
    fi

    if [ $max_attempts -gt 1 ]; then
      echo "${red}------------------ Allocation failed, cleaning up... ------------------${reset}"
    fi

    # Clean up the failed attempt
    local script_dir="$(cd "$(dirname "$0")" && pwd)"
    if [ -x "${script_dir}/delete.sh" ]; then
      "${script_dir}/delete.sh" || true
    fi

    if [ $attempt -lt $max_attempts ]; then
      echo "${yellow}------------------ Sleeping for 5 minutes before retry ------------------${reset}"
      sleep 300
    fi
  done

  echo "${red}------------------ Max retries ($max_attempts) reached, exiting ------------------${reset}"
  exit 1
}

allocate_cluster() {
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
  echo "tkt:  Tekton Pipelines"
  echo ""

  ( set -o pipefail; (serving && dns && networking) 2>&1 | sed  -e 's/^/svr /')&
  ( set -o pipefail; (eventing && namespace) 2>&1 | sed  -e 's/^/evt /')&
  ( set -o pipefail; registry 2>&1 | sed  -e 's/^/reg /') &
  ( set -o pipefail; dapr_runtime 2>&1 | sed  -e 's/^/dpr /')&
  ( set -o pipefail; (tekton && pac) 2>&1 | sed  -e 's/^/tkt /')&

  local job
  for job in $(jobs -p); do
    wait "$job"
  done

  # Configure magic DNS for localtest.me after all services are up
  magic_dns

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
    - containerPort: 443
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
    --patch '{"data":{"localtest.me":""}}' && break

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

arr2yaml() {
  echo -n '['
  local e
  local s=""
  for e in "$@"; do
    printf '%s"%s"' "$s" "$e";
    s=", "
  done
  echo -n ']'
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
  local kind_addr6
  local addr_array

  kind_addr="$($CONTAINER_ENGINE container inspect func-control-plane | jq '.[0].NetworkSettings.Networks.kind.IPAddress' -r)"
  kind_addr6="$($CONTAINER_ENGINE container inspect func-control-plane | jq '.[0].NetworkSettings.Networks.kind.GlobalIPv6Address' -r)"

  addr_array=()

  if [[ -n "$kind_addr" ]]; then
    addr_array+=("$kind_addr/32");
  fi

  if [[ -n "$kind_addr6" ]]; then
    addr_array+=("$kind_addr6/128");
  fi


  echo "Setting up address pool."
  $KUBECTL apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: example
  namespace: metallb-system
spec:
  addresses: $(arr2yaml "${addr_array[@]}")
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

  echo "Patching contour to prefer duals-tack"
  kubectl patch -n contour-external svc/envoy --type merge --patch '{"spec":{"ipFamilyPolicy":"PreferDualStack"}}'

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
  # Deploy Redis using simple manifest with official Redis image
  # (Bitnami images have migration issues as of Sept 2025)
  $KUBECTL apply -f - << EOF
apiVersion: v1
kind: Service
metadata:
  name: redis-master
  namespace: default
spec:
  ports:
  - port: 6379
    targetPort: 6379
  selector:
    app: redis
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-master
  namespace: default
spec:
  serviceName: redis-master
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-storage
          mountPath: /data
  volumeClaimTemplates:
  - metadata:
      name: redis-storage
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 1Gi
EOF

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
    value: ""
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
    value: ""
EOF

  echo "${green}✅ Dapr Runtime${reset}"
}

tekton() {
  echo "${blue}Installing Tekton ${tekton_version} ${reset}"

  tekton_release="previous/${tekton_version}"
  namespace="${NAMESPACE:-default}"

  $KUBECTL apply -f "https://storage.googleapis.com/tekton-releases/pipeline/${tekton_release}/release.yaml"
  sleep 10
  $KUBECTL wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-controller"
  $KUBECTL wait pod --for=condition=Ready --timeout=180s -n tekton-pipelines -l "app=tekton-pipelines-webhook"
  sleep 10

  $KUBECTL create clusterrolebinding "${namespace}:knative-serving-namespaced-admin" --clusterrole=knative-serving-namespaced-admin --serviceaccount="${namespace}:default"

  echo "${green}✅ Tekton${reset}"
}

pac() {
  echo "${blue}Installing PAC (Pipelines-as-Code) ${pac_version} ${reset}"

  local -r pac_ctr_host="${FUNC_INT_PAC_HOST:-pac-ctr.localtest.me}"

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

magic_dns() {
  echo "${blue}Configuring Magic DNS${reset}"

  local cluster_node_addr
  local cluster_node_addr6

  cluster_node_addr="$($CONTAINER_ENGINE container inspect func-control-plane | jq ".[0].NetworkSettings.Networks.kind.IPAddress" -r)"
  cluster_node_addr6="$($CONTAINER_ENGINE container inspect func-control-plane | jq ".[0].NetworkSettings.Networks.kind.GlobalIPv6Address" -r)"

  local a_recs=""
  local aaaa_recs=""

  if [[ -n $cluster_node_addr ]]; then
    a_recs="\
localtest.me.            IN      A       ${cluster_node_addr}\n\
*.localtest.me.          IN      A       ${cluster_node_addr}\n"
  fi

  if [[ -n $cluster_node_addr6 ]]; then
    aaaa_recs="\
localtest.me.            IN      AAAA    ${cluster_node_addr6}\n\
*.localtest.me.          IN      AAAA    ${cluster_node_addr6}\n"
  fi

  $KUBECTL patch cm/coredns -n kube-system --patch-file /dev/stdin <<EOF
{
  "data": {
    "Corefile": ".:53 {\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    file /etc/coredns/example.db localtest.me\n    prometheus :9153\n    forward . /etc/resolv.conf {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
    "example.db": "; localtest.me test file\n\
localtest.me.            IN      SOA     sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600\n\
$a_recs\
$aaaa_recs"
  }
}
EOF

  $KUBECTL patch deploy/coredns -n kube-system --patch-file /dev/stdin <<EOF
{
  "spec": {
    "template": {
      "spec": {
        "\$setElementOrder/volumes": [
          {
            "name": "config-volume"
          }
        ],
        "volumes": [
          {
            "\$retainKeys": [
              "configMap",
              "name"
            ],
            "configMap": {
              "items": [
                {
                  "key": "Corefile",
                  "path": "Corefile"
                },
                {
                  "key": "example.db",
                  "path": "example.db"
                }
              ]
            },
            "name": "config-volume"
          }
        ]
      }
    }
  }
}
EOF
  sleep 1
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=15s

  echo "${green}✅ Magic DNS${reset}"
}

next_steps() {
  echo -e ""
  echo -e "${blue}Next Steps${reset}"
  echo -e "${blue}----------${reset}"
  echo -e ""
  echo -e "${grey}REGISTRY"
  echo -e "Before using the cluster for integration and E2E tests, please run \"${reset}registry.sh${grey}\" (Linux systems) which will configure podman or docker to communicate with the standalone container registry without TLS."
  echo -e ""
  echo -e "For other operating systems, or to do this manually, edit the docker daemon config:"
  echo -e "  - Linux: /etc/docker/daemon.json"
  echo -e "  - macOS: ~/.docker/daemon.json (or via Docker Desktop settings)"
  echo -e "Add the following configuration:"
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
