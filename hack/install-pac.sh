#!/usr/bin/env bash

function install_pac() {
    local -r pac_ctr_host="pac-ctr.127.0.0.1.sslip.io"
    local -r pac_version="v0.17.1"

    # Install Pipelines as Code
    kubectl apply -f "https://raw.githubusercontent.com/openshift-pipelines/pipelines-as-code/release-${pac_version}/release.k8s.yaml"
    sleep 5
    kubectl wait pod --for=condition=Ready -l '!job-name' -n pipelines-as-code --timeout=5m

    # Install ingress for the PaC controller. This is used by VCS Webhooks.
    kubectl apply -f - << EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: pipelines-as-code
  namespace: pipelines-as-code
spec:
  ingressClassName: contour-external
  rules:
  - host: ${pac_ctr_host}
    http:
      paths:
      - backend:
          service:
            name: pipelines-as-code-controller
            port:
              number: 8080
        pathType: Prefix
        path: /
EOF
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
      install_pac
  }
  main "$@"
fi