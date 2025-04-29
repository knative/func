#!/usr/bin/env bash

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# Install GitLab
#

source "$(dirname "$(realpath "$0")")/common.sh"

function install_gitlab() {
  echo "${blue}Installing GitLab${reset}"

  local -r gitlab_host="${GITLAB_HOSTNAME:-gitlab.127.0.0.1.sslip.io}"

  $KUBECTL apply -f - <<EOF
kind: Namespace
apiVersion: v1
metadata:
  name: gitlab
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: gitlab
  namespace: gitlab
  labels:
    app: gitlab
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 2Gi
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
      volumeMounts:
        - name: gitlab
          subPath: config
          mountPath: /etc/gitlab
        - name: gitlab
          subPath: logs
          mountPath: /var/log/gitlab
        - name: gitlab
          subPath: data
          mountPath: /var/opt/gitlab
      env:
        - name: GITLAB_ROOT_PASSWORD
          value: ${GITLAB_ROOT_PASSWORD}
        - name: GITLAB_OMNIBUS_CONFIG
          value: |
            external_url 'http://${gitlab_host}'
            gitlab_rails['gitlab_shell_ssh_port'] = 30022
            gitlab_rails['gitlab_email_enabled'] = false
            puma['worker_processes'] = 0
            prometheus_monitoring['enable'] = false
            gitlab_rails['env'] = {
              'MALLOC_CONF' => 'dirty_decay_ms:1000,muzzy_decay_ms:1000'
            }
            gitaly['configuration'] = {
              ruby_max_rss: 200_000_000,
              concurrency: [
                {
                  rpc: "/gitaly.SmartHTTPService/PostReceivePack",
                  max_per_repo: 1
                }, {
                  rpc: "/gitaly.SSHService/SSHUploadPack",
                  max_per_repo: 1
                }
              ]
            }
            gitaly['env'] = {
              'MALLOC_CONF' => 'dirty_decay_ms:1000,muzzy_decay_ms:1000',
              'GITALY_COMMAND_SPAWN_MAX_PARALLEL' => '1'
            }
      ports:
        - containerPort: 80
          name: http
        - containerPort: 22
          name: ssh
      resources:
        requests:
          memory: "2048Mi"
        limits:
          memory: "4096Mi"
  volumes:
    - name: gitlab
      persistentVolumeClaim:
        claimName: gitlab
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
      port: 30022
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
      port: 30022
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
  $KUBECTL wait pod --for=condition=Ready -l '!job-name' -n gitlab --timeout=5m

  echo '::group::Waiting for Gitlab'
  if ! curl --retry 120 -f --retry-all-errors --retry-delay 5 "${gitlab_host}"; then
    $KUBECTL logs pod/gitlab -n gitlab
    echo '::endgroup::'
    return 1
  fi
  echo
  echo '::endgroup::'
  echo "the GitLab server is available at: http://${gitlab_host}"
  echo "${green}✅ GitLab${reset}"
}

# Invoke only when run directly
# Be a library when sourced
if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    install_gitlab
  }
  main "$@"
fi
