# Create a local kind cluster with
# Knative Serving, and Kourier networking installed.
# Suitable for use locally during development.
# CI/CD uses the very similar knative-kind action

source "$(cd "$(dirname "$0")" && pwd)/common.sh"

delete_resources() {
  echo "${blue}Deleting Cluster${reset}"

  # The in-cluster registry (registry:2 Deployment + NodePort Service) is
  # deleted automatically together with the Kind cluster below.
  $KIND delete cluster --name=func --kubeconfig="${KUBECONFIG}"
  echo "${red}NOTE:${reset}  The following changes have not been undone:"
  echo " - Config setting registry localhost:50000 as insecure (Docker/Podman)"
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
