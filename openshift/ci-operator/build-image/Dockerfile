# DO NOT EDIT! Generated Dockerfile.

FROM registry.ci.openshift.org/ocp/4.17:cli-artifacts as tools

# Dockerfile to bootstrap build and test in openshift-ci
FROM registry.redhat.io/rhel10/go-toolset@sha256:050c556adbf8a684de705abadfdfaf105936e5b4e7abe4f46d6b095a684bfa37 as builder

USER 0:0
RUN yum update -y go
USER 1001:0

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
