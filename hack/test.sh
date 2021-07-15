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
    sleep 5
  done

  sleep 60

  # wait for the route to become ready
  kubectl wait --for=condition=Ready route echo -n func

  echo "${em}  Invoking echo server${me}"
  curl http://echo.func.127.0.0.1.sslip.io/

  echo "${em}DONE${me}"

}

main "$@"


