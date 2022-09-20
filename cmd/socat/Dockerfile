FROM registry.redhat.io/rhel8/go-toolset as builder

COPY main.go /opt/app-root/src/

ENV CGO_ENABLED=0

RUN go mod init go-socat
RUN go get github.com/spf13/cobra@v1.5.0
RUN go build -o socat -ldflags "-s -w" main.go

FROM scratch

COPY --from=builder --chown=1001:1001 /opt/app-root/src/socat /socat

ENV PATH=/

USER 1001:1001

ENTRYPOINT ["/socat"]
