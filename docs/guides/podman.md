# Podman

Under Linux, it is possible to use [podman](https://podman.io/) instead of [docker](https://www.docker.com/).

In order to do this you need to run `podman` as a service. You can do this by following command.
```
❯ podman system service --time=0
```
The command will serve docker API on UNIX socket: `/run/user/{uid}/podman/podman.sock`.

Subsequently, you will need to set environment variable `DOCKER_HOST` to the socket so `func` knows where to connect.
After that you can use `func` as usual.
```
❯ export DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
❯ func build -v
```