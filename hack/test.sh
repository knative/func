#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Testing Cluster...${me}"

  kubectl get services -A
  kubectl get po -A
  kubectl get ingresses.networking.internal.knative.dev -o=custom-columns='NAME:.metadata.name,LABELS:.metadata.labels'
  kubectl get svc -n knative-serving webhook -oyaml
  kubectl delete pod -n knative-serving -lapp=webhook
  sleep 30

  echo "${em}-- creating echo${me}"
  cat <<EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: echo
  namespace: func
spec:
  template:
    spec:
      containers:
        - image: docker.io/jmalloc/echo-server
EOF
  sleep 10
  echo "${em}-- echo created${me}"
  kubectl get services -A
  kubectl get po -A
  sleep 5
  echo "${em}-- invoking echo${me}"
  curl -H "Host: echo.func.cluster.local" http://127.0.0.1/
  kubectl get po --all-namespaces

  echo "${em}DONE${me}"

}


main "$@"


