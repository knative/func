FROM registry.access.redhat.com/ubi8/ubi:8.2 as build
RUN dnf install -y make git gcc gcc-c++ golang
COPY . /src
WORKDIR /src
RUN make

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.2
RUN microdnf install -y ca-certificates
COPY --from=build /src/func /bin/
ENTRYPOINT ["func"]
