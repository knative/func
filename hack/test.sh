#!/usr/bin/env bash

# 
# Runs a test of the knative serving installation by deploying and then 
# invoking an http echoing server.
#

source "$(dirname "$(realpath "$0")")/common.sh"

echo_test() {
  echo "${blue}Testing Cluster via Echo Service${reset}"

  i=0; n=10
  while :; do
    cat <<EOF | kubectl apply -f - && break
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
    (( i+=1 ))
    if (( i>=n )); then
      echo "Unable to create echo service"
      exit 1
    fi
    echo "Retrying..."
    sleep 10
  done

  # Sleep to avoid a racing condition where `kubectl wait` below will fail
  # immediately that the "echo" route is not found and can thus not be waited
  # upon to complete.
  sleep 60

  # Wait for the test to become available
  echo "${blue}Waiting for echo route${reset}"
  kubectl wait --for=condition=Ready route echo -n func --timeout=600s

  echo "${blue}Invoking echo server${reset}"
  curl http://echo.func.127.0.0.1.sslip.io/

  echo "${green}✅ Echo succeeded${reset}"
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    echo_test
  }
  main "$@"
fi


