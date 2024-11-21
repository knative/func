FROM docker.io/library/alpine:latest

RUN --mount=type=cache,target=/tmp/cache/,id=42 echo "Hello!"
