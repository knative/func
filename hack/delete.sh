# Create a local kind cluster with
# Knative Serving, and Kourier networking installed.
# Suitable for use locally during development.
# CI/CD uses the very similar knative-kind action

set -o errexit
set -o nounset
set -o pipefail

export TERM="${TERM:-dumb}"

main() {
  local green=$(tput bold)$(tput setaf 2)
  local red=$(tput bold)$(tput setaf 2)
  local reset=$(tput sgr0)

  echo "${green}Deleting Cluster, Registry and Network${reset}"
  kind delete cluster --name "kind"
  docker stop kind-registry && docker rm kind-registry
  docker network rm kind
  echo "${red}NOTE:${reset}  The following changes have not been undone:"
  echo " - Manual etc/hosts entry for kind-registry"
  echo " - Manual docker config registering kind-registry as insecure"
  echo " - Downloaded container images were not removed"
  echo "${green}DONE${reset}"
}

main
