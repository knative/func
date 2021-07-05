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
  sleep 5

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
  sleep 5
  kubectl get services -A
  kubectl get po -A
  sleep 5
  curl -H "Host: echo.func.cluster.local" http://127.0.0.1/
  kubectl get po --all-namespaces

  echo "${em}DONE${me}"

}


main "$@"


