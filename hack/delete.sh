# Create a local kind cluster with
# Knative Serving, and Kourier networking installed.
# Suitable for use locally during development.
# CI/CD uses the very similar knative-kind action

source "$(dirname "$(realpath "$0")")/common.sh"

delete_resources() {
  echo "${blue}Deleting Cluster and Registry${reset}"

  $KIND delete cluster --name=func --kubeconfig="${KUBECONFIG}"
  docker stop func-registry && docker rm func-registry
  echo "${red}NOTE:${reset}  The following changes have not been undone:"
  echo " - Config setting registry localhost:50000 (func-registry) as insecure"
  echo " - Downloaded container images were not removed"
  echo "${green}DONE${reset}"
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    delete_resources
  }
  main "$@"
fi
