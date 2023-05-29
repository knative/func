#!/usr/bin/env bash

function install_gitlab() {
  local -r gitlab_host="gitlab.127.0.0.1.sslip.io"

  kubectl apply -f - <<EOF
kind: Namespace
apiVersion: v1
metadata:
  name: gitlab
---
apiVersion: v1
kind: Pod
metadata:
  name: gitlab
  namespace: gitlab
  labels:
    app.kubernetes.io/name: gitlab
spec:
  containers:
    - name: gitlab
      image: gitlab/gitlab-ce:latest
      env:
        - name: GITLAB_OMNIBUS_CONFIG
          value: |
            external_url 'http://${gitlab_host}'
            puma['worker_processes'] = 0
            sidekiq['max_concurrency'] = 2
            prometheus_monitoring['enable'] = false
            gitlab_rails['env'] = {
              'MALLOC_CONF' => 'dirty_decay_ms:1000,muzzy_decay_ms:1000'
            }
            gitaly['configuration'] = {
              ruby_max_rss: 200_000_000,
              concurrency: [
                {
                  rpc: "/gitaly.SmartHTTPService/PostReceivePack",
                  max_per_repo: 3
                }, {
                  rpc: "/gitaly.SSHService/SSHUploadPack",
                  max_per_repo: 3
                }
              ]
            }
            gitaly['env'] = {
              'MALLOC_CONF' => 'dirty_decay_ms:1000,muzzy_decay_ms:1000',
              'GITALY_COMMAND_SPAWN_MAX_PARALLEL' => '2'
            }
      ports:
        - containerPort: 80
          name: http
        - containerPort: 22
          name: ssh
---
apiVersion: v1
kind: Service
metadata:
  name: gitlab-internal
  namespace: gitlab
spec:
  selector:
    app.kubernetes.io/name: gitlab
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
    - name: ssh
      protocol: TCP
      port: 22
      targetPort: ssh
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: gitlab-external-ssh
  namespace: gitlab
spec:
  selector:
    app.kubernetes.io/name: gitlab
  ports:
    - name: ssh
      protocol: TCP
      port: 22
      targetPort: ssh
      nodePort: 30022
  type: NodePort
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gitlab
  namespace: gitlab
spec:
  ingressClassName: contour-external
  rules:
    - host: ${gitlab_host}
      http:
        paths:
          - backend:
              service:
                name: gitlab-internal
                port:
                  number: 80
            pathType: Prefix
            path: /

EOF

  sleep 1
  kubectl wait pod --for=condition=Ready -l '!job-name' -n gitlab --timeout=5m

  echo '::group::Waiting for Gitlab'
  curl --retry 60 -f --retry-all-errors --retry-delay 5 "${gitlab_host}"
  echo '::endgroup::'
}

function get_init_root_pwd() {
  kubectl exec -it gitlab -n gitlab -- cat /etc/gitlab/initial_root_password | \
    grep 'Password: ' | \
    cut -f2 -d ':' | \
    tr -d '[:space:]'
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    install_gitlab
  }
  main "$@"
fi