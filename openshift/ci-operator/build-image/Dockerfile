# DO NOT EDIT! Generated Dockerfile.

FROM registry.ci.openshift.org/ocp/4.17:cli-artifacts as tools

# Dockerfile to bootstrap build and test in openshift-ci
FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.24-nofips-openshift-4.19 as builder

ARG TARGETARCH

COPY --from=tools /usr/share/openshift/linux_$TARGETARCH/oc.rhel8 /usr/bin/oc

RUN ln -s /usr/bin/oc /usr/bin/kubectl

RUN yum install -y httpd-tools

RUN wget https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 ./get-helm-3

RUN ./get-helm-3 --version v3.11.3 --no-sudo && helm version

RUN GOFLAGS='' go install github.com/mikefarah/yq/v3@latest
RUN GOFLAGS='' go install -tags="exclude_graphdriver_btrfs containers_image_openpgp" github.com/containers/skopeo/cmd/skopeo@v1.17.0

# go install creates $GOPATH/.cache with root permissions, we delete it here
# to avoid permission issues with the runtime users
RUN rm -rf $GOPATH/.cache
