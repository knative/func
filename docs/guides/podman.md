# Podman

Under Linux, it is possible to use [podman](https://podman.io/) instead of [docker](https://www.docker.com/). With Functions, you'll need `podman` version `v3.3` or better for this to work properly.

For `func` version `v0.20.0` and later on Linux no further setup is needed,
`func` should use `podman` automatically.

For older versions or if you use Windows or macOS some setup is required:

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
❯ export DOCKER_HOST=$(podman system connection ls --format="{{.URI}}" | grep root)
```

### Remote connections

You may also connect to a remote `podman` running on a remote Linux server. Follow the instructions above for running `podman` as a service on a Linux server. You will connect to this via SSH when `func` builds a project. In addition to having `podman` on the remote Linux server, you will also need to run the SSH daemon. Installing, configuring and running SSH is outside of the scope of this project. Please work with a system administrator to ensure that this prerequisite is met.

To use `podman` remotely, create (or reuse) a default key on your client MacOS computer. If you do not already have a key created, you can create one with the following command.

```
ssh-keygen -t ed255519
```

This will create a public, private key pair. By default, if you followed the command above, the public key will be in `~/.ssh/id_ed255519.pub`. Add the contents of this file to the `authorized_keys` file on the remote Linux server. This should conclude the SSH configuration required.

Once you have SSH configured, you need to configure `podman` on the local macOS computer. To add the remote connection to `podman` you will need to know the IP address or hostname of the Linux server that you are connecting to. You will also need to know the location of the unix socket used by `podman`. To find the location, you can issue the following command on the Linux server.

```
podman info | grep podman.sock
```

This should display something like `path: /run/user/1000/podman/podman.sock`. When you have this path, and the hostname or IP address of the Linux server, you can configure `podman` with the following command.

```
podman --remote system connection add podman-remote --identity ~/.ssh/id_ed255519 ssh://username@hostname/path/to/podman.socket
```

In the example above, a connection called `podman-remote` is added, which will connect using the SSH key created earlier to the host at `hostname` as the user `username`. Here is a real world example.

```
podman --remote system connection add podman-remote --identity ~/.ssh/id_podman_client ssh://lanceball@192.168.1.203/run/user/1000/podman/podman.sock
```

To make the newly configured remote the default for `podman` on your laptop, run the following command.

```
podman --remote system connection default podman-remote
```

To check if it was successfully added, run the following command. You should see the `podman-remote` connection added.

```
❯ podman --remote system connection list
Name                         Identity                                  URI
podman-remote*               /Users/lball/.ssh/id_podman_client        ssh://lanceball@192.168.1.203:22/run/user/1000/podman/podman.sock
```

Finally, to ensure that `func` knows the address of your newly configured registry, set and export the `DOCKER_HOST` environment variable with the following command.

```
export DOCKER_HOST=$(podman system connection ls --format="{{.URI}}" | head -1)
```


## Known issues

* In func version 0.19, some runtimes might be unable to build a function by using podman. You might see an error message similar to the following:

    ```
    ERROR: failed to image: error during connect: Get "http://%2Fvar%2Frun%2Fdocker.sock/v1.40/info": EOF
    ```
  * The following workaround exists for this issue:
    * Update the podman service by adding --time=0 to the service ExecStart definition in the podman configuration file at `/etc/systemd/user/podman.service`.You might need to copy the default configuration file from `/usr/lib/systemd/user/podman.service` first.
      </br></br>
      Example service configuration:

      `ExecStart=/usr/bin/podman $LOGGING system service --time=0`
      </br></br>
      After editing the configuration files you need to reload/restart service:
      ```
      ❯ systemctl --user daemon-reload
      ❯ systemctl restart --user podman.socket
      ```
