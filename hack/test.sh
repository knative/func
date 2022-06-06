#!/usr/bin/env bash

# 
# Runs a test of the knative serving installation by deploying and then 
# invoking an http echoing server.
#

set -o errexit
set -o nounset
set -o pipefail

export TERM="${TERM:-dumb}"

main() {
  echo "TERM: $TERM"
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  # Drop some debug in the event even the above excessive wait does not work.
  echo "${em}Testing...${me}"

  echo "${em}  Creating echo server${me}"

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
  sleep 30

  # Wait for the test to become available
  echo "${em}  Waiting for echo route${me}"
  kubectl wait --for=condition=Ready route echo -n func --timeout=120s

  echo "${em}  Invoking echo server${me}"
  curl http://echo.func.127.0.0.1.sslip.io/

  echo "${em}DONE${me}"

}

main "$@"


