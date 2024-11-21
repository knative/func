FROM scratch

RUN --mount=type=cache,target=/tmp/cache/,id=42 echo "Hello!"
