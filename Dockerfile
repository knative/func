FROM golang:alpine as build
RUN apk add make git gcc g++ 
COPY . /src
WORKDIR /src
RUN make

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /src/faas /bin/
ENTRYPOINT ["faas"]
