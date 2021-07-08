#!/usr/bin/env bash

# 
# Runs a test of the knative serving installation by deploying and then 
# invoking an http echoing server.
#

set -o errexit
set -o nounset
set -o pipefail

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Waiting for stability...${me}"
  # Sadly this long wait appears to be necessary as of recent
  # versions due to restarts of kourier gateway and knative activator. The
  # root cause is currently unknown, but it eventually starts.  Eventually.
  sleep 420

  # Drop some debug in the event even the above excessive wait does not work.
  kubectl get services -A
  kubectl get po -A
  echo "==== Activator:"
  kubectl describe po -lapp=activator -n knative-serving
  kubectl logs -lapp=activator -n knative-serving
  echo "==== Gateway:"
  kubectl describe po -n kourier-system -lapp=3scale-kourier-gateway
  kubectl logs -n kourier-system -lapp=3scale-kourier-gateway

  echo "${em}Testing...${me}"

  echo "${em}-- creating echo server${me}"
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
  sleep 20
  echo "${em}-- invoking echo server${me}"
  curl http://echo.func.127.0.0.1.sslip.io/ 

  echo "${em}DONE${me}"

}

main "$@"


