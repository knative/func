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

  echo "${green}Deleting Cluster and Registry${reset}"
  kind delete cluster --name "func"
  docker stop func-registry && docker rm func-registry
  echo "${red}NOTE:${reset}  The following changes have not been undone:"
  echo " - Config setting registry localhost:50000 (func-registry) as insecure"
  echo " - Downloaded container images were not removed"
  echo "${green}DONE${reset}"
}

main
