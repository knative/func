# DO NOT EDIT! Generated Dockerfile for cmd/func-util.
ARG GO_BUILDER=registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.22-openshift-4.17
ARG GO_RUNTIME=registry.access.redhat.com/ubi8/ubi-minimal

FROM $GO_BUILDER as builder

COPY . .

ENV CGO_ENABLED=1
ENV GOEXPERIMENT=strictfipsruntime

RUN go build -tags strictfipsruntime -o /usr/bin/main ./cmd/func-util

FROM $GO_RUNTIME

ARG VERSION=knative-nightly

RUN microdnf install tzdata socat tar

USER 65532

COPY --from=builder /usr/bin/main /usr/bin/func-util

RUN ln -s /usr/bin/func-util /usr/local/bin/deploy && \
	ln -s /usr/bin/func-util /usr/local/bin/scaffold && \
	ln -s /usr/bin/func-util /usr/local/bin/s2i

LABEL \
      com.redhat.component="openshift-serverless-1-kn-plugin-func-func-util-rhel8-container" \
      name="openshift-serverless-1/kn-plugin-func-func-util-rhel8" \
      version=$VERSION \
      summary="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util" \
      maintainer="serverless-support@redhat.com" \
      description="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util" \
      io.k8s.display-name="Red Hat OpenShift Serverless 1 Kn Plugin Func Func Util"

ENTRYPOINT ["/usr/bin/func-util"]
