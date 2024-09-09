# DO NOT EDIT! Generated Dockerfile.

# Dockerfile to bootstrap build and test in openshift-ci
FROM registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.22-openshift-4.17 as builder

RUN echo "[kubernetes]" >> /etc/yum.repos.d/kubernetes.repo && \
    echo "name=Kubernetes" >> /etc/yum.repos.d/kubernetes.repo && \
    echo "baseurl=https://pkgs.k8s.io/core:/stable:/v1.29/rpm/" >> /etc/yum.repos.d/kubernetes.repo && \
    echo "enabled=1" >> /etc/yum.repos.d/kubernetes.repo && \
    echo "gpgcheck=1" >> /etc/yum.repos.d/kubernetes.repo && \
    echo "gpgkey=https://pkgs.k8s.io/core:/stable:/v1.29/rpm/repodata/repomd.xml.key" >> /etc/yum.repos.d/kubernetes.repo

RUN yum install -y kubectl httpd-tools

RUN wget https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 ./get-helm-3

RUN ./get-helm-3 --version v3.11.3 --no-sudo && helm version

RUN GOFLAGS='' go install github.com/mikefarah/yq/v3@latest

# go install creates $GOPATH/.cache with root permissions, we delete it here
# to avoid permission issues with the runtime users
RUN rm -rf $GOPATH/.cache

# Allow runtime users to add entries to /etc/passwd
RUN chmod g+rw /etc/passwd
