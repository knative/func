# CLI Commands

## `create`

Creates a new Function project at _`path`_. If _`path`_ is unspecified, assumes the current directory. If _`path`_ does not exist, it will be created. The function name is the name of the leaf directory at path. The user can specify the runtime and trigger with flags.

Similar `kn` command: none.

```console
function create <path> [-l <runtime> -t <trigger>]
```

When run as a `kn` plugin.

```console
kn function create <path> [-l <runtime> -t <trigger>]
```

## `build`

Builds the Function project in the current directory. Reads the `function.yaml` file to determine image name and registry. If both of these values are unset in the configuration file, the user is prompted to provide a registry, from there an image name can be derived. The image name and registry may also be specified as flags, as can the path to the project.

The value(s) provided for image and registry are persisted to the `function.yaml` file so that subsequent invocations do not require the user to specify these again.

Similar `kn` command: none.

```console
function build [-i <image> -r <registry> -p <path>]
```

When run as a `kn` plugin.

```console
kn function build [-i <image> -r <registry> -p <path>]
```

## `run`

Runs the Function project locally in the container. If a container has not yet been created, prompts the user to run `function build`.  The user may specify a path to the project directory using the `--path` or `-p` flag.

Similar `kn` command: none.

```console
function run
```

When run as a `kn` plugin.

```console
kn function run [-p <path>]
```

## `deploy`

Builds and deploys the Function project in the current directory. The user may specify a path to the project directory using the `--path` or `-p` flag. Reads the `function.yaml` configuration file to determine the image name. An image and registry may be specified on the command line using the  `--image` or `-i` and `--registry` or `-r` flag.

Derives the service name from the project name. There is no mechanism by which the user can specify the service name. The user must have already initialized the  function using `function create` or they will encounter an error.

If the Function is already deployed, it is updated with a new container image that is pushed to a
container image registry, and the Knative Service is updated.

The namespace into which the project is deployed defaults to the value in the `function.yaml` configuration file. If `NAMESPACE` is not set in the configuration, the namespace currently active in the Kubernetes configuration file will be used. The namespace may be specified on the command line using the `--namespace` or `-n` flag, and if so this will overwrite the value in the `function.yaml` file.

Similar `kn` command: `kn service create NAME --image IMAGE [flags]`. This command allows a user to deploy a Knative Service by specifying an image, typically one hosted on a public container registry such as docker.io. The deployment options which the `kn` command affords the user are quite broad. The `kn` command in this case is quite effective for a power user. The `function deploy` command has a similar end result, but is definitely easier for a user just getting started to be successful with.

```console
function deploy [-n <namespace> -p <path> -i <image> -r <registry>]
```

When run as a `kn` plugin.

```console
kn function deploy [-n <namespace> -p <path> -i <image> -r <registry>]
```

## `describe`

Prints the name, route and any event subscriptions for a deployed Function. The user may also specify the name of the function to describe. The namespace defaults to the value in `function.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `function.yaml`.

Similar `kn` command: `kn service describe NAME [flags]`. This flag provides a lot of nice information not available in `function describe`, such as revisions, age, annotations and labels. This command should be renamed to make it distinct from `kn` - e.g. `function status`.

```console
function describe [-f <format> -n <namespace> -p <path>]
```

When run as a `kn` plugin.

```console
kn function describe [-f <format> -n <namespace> -p <path>]
```

## `list`

Lists all deployed functions. The namespace defaults to the value in `function.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace defaults to the value in `function.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `function.yaml`.

Similar `kn` command: `kn service list [name] [flags]`. This command lists all deployed Knative `Services`. As with other `kn` commands that have similar functionality, there is more information and flexibilty in the `kn` command. However, `kn` will return _all_ `Services`, while `function list` will only display the boson Functions that have been deployed. Consider improving the output of the `function list` command so that it is at least as informative as `kn service list`.

```console
function list [-n <namespace> -p <path>]
```

When run as a `kn` plugin.

```console
kn function list [-n <namespace> -p <path>]
```

## `delete`

Removes a deployed function from the cluster. The user may specify a function by name, path or if neither of those are provided, the current directory will be searched for a `function.yaml` configuration file to determine the function to be removed. The namespace defaults to the value in `function.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `function.yaml`.

Similar `kn` command: `kn service delete NAME [flags]`.

```console
function delete <name> [-n namespace, -p path]
```

When run as a `kn` plugin.

```console
kn function delete <name> [-n namespace, -p path]
```
