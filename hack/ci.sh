#!/usr/bin/env bash

# 
# CI-specific configuration for linux systems.
# Patches docker to allow the local kind registtry without authentication.
# Adds the registry to the local hosts file
# Restarts the internal webkook (fix).
#

set -o errexit
set -o nounset
set -o pipefail

main() {
  local em=$(tput bold)$(tput setaf 2)
  local me=$(tput sgr0)

  echo "${em}Configuring for CI...${me}"

  set_registry_insecure
  patch_hosts
  fix_webhook

  echo "${em}DONE${me}"

}

set_registry_insecure() {
    echo 'Setting registry as trusted local-only'
    patch=".\"insecure-registries\" = [\"kind-registry:5000\""]
    sudo jq "$patch" /etc/docker/daemon.json > /tmp/daemon.json.tmp && sudo mv /tmp/daemon.json.tmp /etc/docker/daemon.json
    sudo service docker restart
}

patch_hosts() {
    echo 'Adding registry to hosts'
    echo "127.0.0.1 kind-registry" | sudo tee --append /etc/hosts
}

fix_webhook() {
  kubectl get svc -n knative-serving webhook -oyaml
  kubectl delete pod -n knative-serving -lapp=webhook
  sleep 10
  kubectl get pod -n knative-serving -lapp=webhook -oyaml
}

main "$@"

