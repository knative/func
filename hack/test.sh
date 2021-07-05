#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Testing Cluster...${me}"

  kubectl get svc -n knative-serving webhook -oyaml
  kubectl delete pod -n knative-serving -lapp=webhook
  sleep 20
  kubectl get pod -n knative-serving -lapp=webhook -oyaml


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
  echo "${em}-- invoking echo${me}"
  curl -H "Host: echo.func.cluster.local" http://127.0.0.1/

  echo "${em}DONE${me}"

}

main "$@"


