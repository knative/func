# CLI Commands

## `create`

Creates a new Function project at _`path`_. If _`path`_ is unspecified, assumes the current directory. If _`path`_ does not exist, it will be created. The function name is the name of the leaf directory at path. The user can specify the runtime and template with flags.

Function name must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?').

The files written upon create include an example Function of the specified language runtime, example tests, and a metadata file `func.yaml`.  Together, these are referred to as a Template.  Included are the templates 'http' and 'cloudevents' (default is 'http') for each language runtime.  A template can be pulled from a specific Git repository by providing the `--repository` flag, or from a locally installed repository using the repository's name as a prefix.  See the [Templates Guide](templates.md) for more information.

Similar `kn` command: none.

```console
func create <path> [-l <runtime> -t <template> -r <repository>]
```

When run as a `kn` plugin.

```console
kn func create <path> [-l <runtime> -t <template> -r <repository>]
```

## `build`

Builds the Function project in the current directory. Reads the `func.yaml` file to determine image name and registry. If both of these values are unset in the configuration file, the user is prompted to provide a registry, from there an image name can be derived. The image name and registry may also be specified as flags, as can the path to the project.

The value(s) provided for image and registry are persisted to the `func.yaml` file so that subsequent invocations do not require the user to specify these again.

Similar `kn` command: none.

```console
func build [-i <image> -r <registry> -p <path>]
```

When run as a `kn` plugin.

```console
kn func build [-i <image> -r <registry> -p <path>]
```

## `run`

Runs the Function project locally in the container. If a container has not yet been created, prompts the user to run `func build`.  The user may specify a path to the project directory using the `--path` or `-p` flag. The user may set an environment variable by using `--env` or `-e` flag, e.g. `-e VAR_NAME=VAR_VALUE`. To unset a variable dash `-` suffix is used, e.g. `-e VAR_NAME-`.

Similar `kn` command: none.

```console
func run
```

When run as a `kn` plugin.

```console
kn func run [-p <path>]
```

## `deploy`

Deploys the Function project in the current directory. The user may specify a path to the project directory using the `--path` or `-p` flag. Reads the `func.yaml` configuration file to determine the image name. An image and registry may be specified on the command line using the  `--image` or `-i` and `--registry` or `-r` flag. The user may set an environment variable by using `--env` or `-e` flag, e.g. `-e VAR_NAME=VAR_VALUE`. To unset a variable dash `-` suffix is used, e.g. `-e VAR_NAME-`.

Derives the service name from the project name. There is no mechanism by which the user can specify the service name. The user must have already initialized the  function using `func create` or they will encounter an error.

If the Function is already deployed, it is updated with a new container image that is pushed to a
container image registry, and the Knative Service is updated.

By default the Function image to be deployed is also built.  The build can be skipped by specifying `--build=false`.

The namespace into which the project is deployed defaults to the value in the `func.yaml` configuration file. If `NAMESPACE` is not set in the configuration, the namespace currently active in the Kubernetes configuration file will be used. The namespace may be specified on the command line using the `--namespace` or `-n` flag, and if so this will overwrite the value in the `func.yaml` file.

Similar `kn` command: `kn service create NAME --image IMAGE [flags]`. This command allows a user to deploy a Knative Service by specifying an image, typically one hosted on a public container registry such as docker.io. The deployment options which the `kn` command affords the user are quite broad. The `kn` command in this case is quite effective for a power user. The `func deploy` command has a similar end result, but is definitely easier for a user just getting started to be successful with.

```console
func deploy [-n <namespace> -p <path> -i <image> -r <registry> -b=true|false]
```

When run as a `kn` plugin.

```console
kn func deploy [-n <namespace> -p <path> -i <image> -r <registry> -b=true|false]
```

## `info`

Prints the name, route and any event subscriptions for a deployed Function. The user may also specify the name of the function to describe. The namespace defaults to the value in `func.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `func.yaml`.

Similar `kn` command: `kn service describe NAME [flags]`. This flag provides a lot of nice information not available in `func info`, such as revisions, age, annotations and labels.

```console
func info [-o <output> -n <namespace> -p <path>]
```

When run as a `kn` plugin.

```console
kn func info [-o <output> -n <namespace> -p <path>]
```

## `list`

Lists all deployed functions. The namespace defaults to the value in `func.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace defaults to the value in `func.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `func.yaml`.

Similar `kn` command: `kn service list [name] [flags]`. This command lists all deployed Knative `Services`. As with other `kn` commands that have similar functionality, there is more information and flexibilty in the `kn` command. However, `kn` will return _all_ `Services`, while `func list` will only display the boson functions that have been deployed. Consider improving the output of the `func list` command so that it is at least as informative as `kn service list`.

```console
func list [-n <namespace> -p <path>]
```

When run as a `kn` plugin.

```console
kn func list [-n <namespace> -p <path>]
```

## `delete`

Removes a deployed function from the cluster. The user may specify a function by name, path. If both of those are provided the command will not be executed and user will receive an error message. If neither of those are provided, the current directory will be searched for a `func.yaml` configuration file to determine the function to be removed. The namespace defaults to the value in `func.yaml` or the namespace currently active in the user's Kubernetes configuration. The namespace may be specified on the command line, and if so this will overwrite the value in `func.yaml`.

Similar `kn` command: `kn service delete NAME [flags]`.

```console
func delete <name> [-n namespace, -p path]
```

When run as a `kn` plugin.

```console
kn func delete <name> [-n namespace, -p path]
```

## `emit`

Emits a CloudEvent, sending it to the deployed function. The user may specify the event type, source and ID,
and may provide event data on the command line or in a file on disk. By default, `event` works on the local
directory, assuming that it is a function project. Alternatively the user may provide a path to a project
directory using the `--path` flag, or send an event to an arbitrary endpoint using the `--sink` flag. The
`--sink` flag also accepts the special value `local` to send an event to the function running locally, for
example, when run via `func run`.

Similar `kn` command when using the [kn-plugin-event](https://github.com/knative-sandbox/kn-plugin-event): `kn event send [FLAGS]`

Examples:

```console
# Send a CloudEvent to the deployed function with no data and default values
# for source, type and ID
kn func emit

# Send a CloudEvent to the deployed function with the data found in ./test.json
kn func emit --file ./test.json

# Send a CloudEvent to the function running locally with a CloudEvent containing
# "Hello World!" as the data field, with a content type of "text/plain"
kn func emit --data "Hello World!" --content-type "text/plain" -s local

# Send a CloudEvent to the function running locally with an event type of "my.event"
kn func emit --type my.event --sink local

# Send a CloudEvent to the deployed function found at /path/to/fn with an id of "fn.test"
kn func emit --path /path/to/fn -i fn.test

# Send a CloudEvent to an arbitrary endpoint
kn func emit --sink "http://my.event.broker.com"
```

## `config`

Invokes interactive prompt that manages configuration of the Function project in the current directory. 
The user may specify a path to the project directory using the `--path` or `-p` flag. This command operates on configuration
specified in `func.yaml` configuration file.
Users need to deploy or update the function with `func deploy` in order to apply the updated configuration to the deployed function.

This command has subcommands `envs` and `volumes` to manage directly the specific resouces: Environment variables and Volumes.
These subcommands has commands `add` and `remove` to add and remove specified resouces.

Invokes top level interactive prompt that allows choosing the resouce and operation:
```console
func config [-p <path>]
```

Example:
```console
func config
? What do you want to configure? Volumes
? What operation do you want to perform? List
Configured Volumes mounts:
 -  Secret "mysecret" mounted at path: "/workspace/secret"
 -  ConfigMap "mycm" mounted at path: "/workspace/configmap"
```

### `config envs`

This command lists configured Environment variables:
```console
func config envs [-p <path>]
```

Invokes interactive prompt to add Environment variables to the function configuration
```console
func config envs add [-p <path>]
```

Invokes interactive prompt to remove Environment variables from the function configuration
```console
func config envs remove [-p <path>]
```

### `config volumes`

This command lists configured Volumes:
```console
func config volumes [-p <path>]
```

Invokes interactive prompt to add Volumes to the function configuration
```console
func config volumes add [-p <path>]
```

Invokes interactive prompt to remove Volumes from the function configuration
```console
func config volumes remove [-p <path>]
```

## `repository`

Manage set of installed repositories.

With no arguments, the help text is shown.
To run using an interactive prompt, use the use the --confirm (-c) flag.
```console
$ func repository -c
```

Manages template repositories installed on disk at either the default location
(~/.config/func/repositories) or the location specified by the --repository
flag.  Once added, a template from the repository can be used when creating
a new Function.

_Alternative Repositories Location:_
Repositories are stored on disk in ~/.config/func/repositories by default.
This location can be altered by either setting the FUNC_REPOSITORIES
environment variable, or by providing the --repositories (-r) flag to any
of the commands.  XDG_CONFIG_HOME is respected when determining the default.

_Interactive Prompts:_
To complete these commands interactively, pass the --confirm (-c) flag to
the 'repository' command, or any of the inidivual subcommands.

_The Default Repository:_
The default repository is not stored on disk, but embedded in the binary and
can be used without explicitly specifying the name.  The default repository is
always listed first, and is assumed when creating a new Function without
specifying a repository name prefix.  For example, to create a new Go function
using the 'http' template from the default repository.
```console
$ func create -l go -t http
```

_The Repository Flag:_
Installing repositories locally is optional.  To use a template from a remote
repository directly, it is possible to use the --repository flag on create.
This leaves the local disk untouched.  For example, To create a Function using
the Boson Project Hello-World template without installing the template
repository locally, use the --repository (-r) flag on create:
```console
$ func create -l go \
--template hello-world \
--repository https://github.com/boson-project/templates
```

### `add`
Add a new repository to the installed set.
```console
$ func repository add <name> <URL>
```

For Example, to add the Boson Project repository:
```console
$ func repository add boson https://github.com/boson-project/templates
```

Once added, a Function can be created with templates from the new repository
by prefixing the template name with the repository.  For example, to create
a new Function using the Go Hello World template:
```console
$ func create -l go -t boson/hello-world
```

### `list`

List all available repositories, including the installed default repository.
Repositories available are listed by name.  To see the URL which was used to
install remotes, use --verbose (-v).

### `rename`

Rename a previously installed repository from <old> to <new>. Only installed
repositories can be renamed.
```console
$ func repository rename <name> <new name>
```

### `remove`

Remove a repository by name.  Removes the repository from local storage
entirely.  When in confirm mode (--confirm) it will confirm before
deletion, but in regular mode this is done immediately, so please use
caution, especially when using an altered repositories location
(FUNC_REPOSITORIES environment variable or --repositories).
```console
$ func repository remove <name>
```

### Examples

o Run in confirmation mode (interactive prompts) using the --confirm flag
```console
$ func repository -c
```

o Add a repository and create a new Function using a template from it:
```console
$ func repository add boson https://github.com/boson-project/templates
$ func repository list
default
boson
$ func create -l go -t boson/hello-world
...
```

o List all repositories including the URL from which remotes were installed
```console
$ func repository list -v
default
boson	https://github.com/boson-project/templates
```

o Rename an installed repository
```console
$ func repository list
default
boson
$ func repository rename boson boson-examples
$ func repository list
default
boson-examples
```

o Remove an installed repository
```console
$ func repository list
default
boson-examples
$ func repository remove boson-examples
$ func repository list
default
```
