# DO NOT EDIT! Generated Dockerfile for cmd/func-util.
ARG GO_BUILDER=registry.redhat.io/rhel10/go-toolset@sha256:050c556adbf8a684de705abadfdfaf105936e5b4e7abe4f46d6b095a684bfa37
ARG GO_RUNTIME=registry.redhat.io/ubi10-minimal@sha256:da1aedf263c25c4d5a58bc9d44a6b9f973a217ed67cb7ad2e08ec2711e901eec

FROM $GO_BUILDER as builder

USER 0:0
RUN yum update -y go
USER 1001:0

WORKDIR /workspace
COPY . .

ENV CGO_ENABLED=1
ENV GOEXPERIMENT=strictfipsruntime
ENV GOFLAGS=''

RUN go build -tags strictfipsruntime,exclude_graphdriver_btrfs -o /usr/bin/main ./cmd/func-util

FROM $GO_RUNTIME

ARG VERSION=knative-nightly

RUN microdnf install socat tar

COPY --from=builder /usr/bin/main /usr/bin/func-util
COPY LICENSE /licenses/

RUN ln -s /usr/bin/func-util /usr/local/bin/deploy && \
	ln -s /usr/bin/func-util /usr/local/bin/scaffold && \
	ln -s /usr/bin/func-util /usr/local/bin/s2i && \
	ln -s /usr/bin/func-util /usr/local/bin/s2i-generate

USER 65532

LABEL \
      com.redhat.component="openshift-serverless-1-kn-plugin-func-func-util-rhel8-container" \
      name="openshift-serverless-1/kn-plugin-func-func-util-rhel8" \
      version=$VERSION \
      summary="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util" \
      maintainer="serverless-support@redhat.com" \
      description="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util" \
      io.k8s.display-name="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util" \
      io.k8s.description="Red Hat OpenShift Serverless Kn Plugin Func Func Util" \
      io.openshift.tags="func-util"

ENTRYPOINT ["/usr/bin/bash"]
