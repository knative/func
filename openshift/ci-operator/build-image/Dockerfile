# DO NOT EDIT! Generated Dockerfile.

FROM registry.ci.openshift.org/ocp/4.17:cli-artifacts as tools

# Dockerfile to bootstrap build and test in openshift-ci
FROM registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.22-openshift-4.17 as builder

ARG TARGETARCH

COPY --from=tools /usr/share/openshift/linux_$TARGETARCH/oc.rhel8 /usr/bin/oc

RUN ln -s /usr/bin/oc /usr/bin/kubectl

RUN yum install -y httpd-tools

RUN wget https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 ./get-helm-3

RUN ./get-helm-3 --version v3.11.3 --no-sudo && helm version

RUN GOFLAGS='' go install github.com/mikefarah/yq/v3@latest

# go install creates $GOPATH/.cache with root permissions, we delete it here
# to avoid permission issues with the runtime users
RUN rm -rf $GOPATH/.cache
