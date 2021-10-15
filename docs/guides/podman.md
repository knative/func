# Podman

Under Linux, it is possible to use [podman](https://podman.io/) instead of [docker](https://www.docker.com/).

In order to do this you need to run `podman` as a service. You can do this with the following command.
```
❯ systemctl start --user podman.socket
```
This will serve the Docker API on a UNIX socket at `${XDG_RUNTIME_DIR}/podman/podman.sock` (on most systems that would be `/run/user/{uid}/podman/podman.sock`).

Then set the environment variable `DOCKER_HOST` to the socket so `func` knows where to connect.
```
❯ export DOCKER_HOST="unix://${XDG_RUNTIME_DIR}/podman/podman.sock"
❯ func build -v
```
Now you may use `func` as usual.
