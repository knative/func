## func run

Run the function locally

### Synopsis


NAME
	func run - Run a function locally

SYNOPSIS
	func run [-r|--registry] [-i|--image] [-e|--env] [--build]
				 [-b|--builder] [--builder-image] [-c|--confirm]
	             [--address] [--json] [-v|--verbose]

DESCRIPTION
	Run the function locally.

	Values provided for flags are not persisted to the function's metadata.

	Containerized Runs
	  You can build your function in a container using the Pack or S2i builders.
	  On the contrary, non-containerized run is achieved via Host builder which
	  will use your host OS' environment to build the function. This builder is
	  currently enabled for Go and Python. Building defaults to using the Host
	  builder when available. You can alter this by using the --builder flag
	  eg: --builder=s2i.

	Process Scaffolding
	  This is an Experimental Feature currently available only to Go and Python
	  projects. When running a function with --builder=host (default when
	  available), the function is first wrapped with code which presents it as
	  a process. This "scaffolding" is transient, written for each build or
	  run, and should in most cases be transparent to a function author.

EXAMPLES

	o Run the function locally using the runtime's default container.
	  $ func run

	o Run the function locally, forcing a rebuild
	  of the container even if no filesysem changes are detected
	  $ func run --build

	o Run the function locally on the host with no containerization (Go/Python only).
	  $ func run --builder=host

	o Run the function locally on a specific address.
	  $ func run --address='[::]:8081'

	o Run the function locally and output JSON with the service address.
	  $ func run --json


```
func run
```

### Options

```
      --address string          Interface and port on which to bind and listen. Default is 127.0.0.1:8080, or an available port if 8080 is not available. ($FUNC_ADDRESS)
      --base-image string       Override the base image for your function (host builder only)
      --build string[="true"]   Build the function. [auto|true|false]. ($FUNC_BUILD) (default "auto")
  -b, --builder string          Builder to use when creating the function's container. Currently supported builders are "host", "pack" and "s2i". Defaults to 'host' for python/go, otherwise 'pack'. (default "pack")
      --builder-image string    Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)
  -c, --confirm                 Prompt to confirm options interactively ($FUNC_CONFIRM)
  -e, --env stringArray         Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables. To unset, specify the environment variable name followed by a "-" (e.g., NAME-).
  -h, --help                    help for run
  -i, --image string            Full image name in the form [registry]/[namespace]/[name]:[tag]. This option takes precedence over --registry. Specifying tag is optional. ($FUNC_IMAGE)
      --json                    Output as JSON. ($FUNC_JSON)
  -p, --path string             Path to the function.  Default is current directory ($FUNC_PATH)
  -r, --registry string         Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)
  -v, --verbose                 Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

