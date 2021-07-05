#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Testing Cluster...${me}"

  # TODO
  kubectl apply -f echo-server.yaml
  sleep 30
  kubectl get services -n func
  sleep 5
  curl -H "Host: echo.func.cluster.local" http://127.0.0.1/
  kubectl get po --all-namespaces

  echo "${em}DONE${me}"

}


main "$@"


