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

## macOS

Under macOS you need to use `podman machine` as `podman` is native to Linux.

```
❯ brew install podman
❯ podman machine init --cpus=2 --disk-size=30 --memory=8192
❯ podman machine start
❯ ssh-add -k $HOME/.ssh/podman-machine-default
❯ export DOCKER_HOST=(podman system connection ls --format="{{.URI}}" | grep root)
```

## Known issues

* In func version 0.19, some runtimes might be unable to build a function by using podman. You might see an error message similar to the following:

    ```
    ERROR: failed to image: error during connect: Get "http://%2Fvar%2Frun%2Fdocker.sock/v1.40/info": EOF
    ```
  * The following workaround exists for this issue:
    * Update the podman service by adding --time=0 to the service ExecStart definition.  The configuration files are: `/etc/systemd/[user|system]/podman.service`.You might need to copy default vendor configuration files from `/usr/lib/systemd/[user|system]/podman.service` first.
      </br></br> 
      Example service configuration:
    
      `ExecStart=/usr/bin/podman $LOGGING system service --time=0`
      </br></br>
      After editing the configuration files you need to reload/restart service:
      ```
      ❯ systemctl --user daemon-reload
      ❯ systemctl restart --user podman.socket
      ```
