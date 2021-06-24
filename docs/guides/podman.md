# Podman

Under Linux, it is possible to use [podman](https://podman.io/) instead of [docker](https://www.docker.com/).

In order to do this you need to run `podman` as a service. You can do this with the following command.
```
❯ podman system service --time=0
```
This will serve the Docker API on a UNIX socket at `/run/user/{uid}/podman/podman.sock`.

Then set the environment variable `DOCKER_HOST` to the socket so `func` knows where to connect.
```
❯ export DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
❯ func build -v
```
Now you may use `func` as usual.
