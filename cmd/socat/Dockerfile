FROM registry.redhat.io/rhel8/go-toolset as builder

COPY go.mod go.sum main.go /opt/app-root/src/

RUN CGO_ENABLED=0 go build -o socat -ldflags "-s -w" main.go

FROM scratch

COPY --from=builder --chown=1001:1001 /opt/app-root/src/socat /socat

ENV PATH=/

USER 1001:1001

ENTRYPOINT ["/socat"]
