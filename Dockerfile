FROM docker.io/library/golang

RUN --mount=type=cache,target=/tmp/cache/,id=42 echo "Hello!"
