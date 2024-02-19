## func run

Run the function locally

### Synopsis


NAME
	func run - Run a function locally

SYNOPSIS
	func run [-t|--container] [-r|--registry] [-i|--image] [-e|--env]
	             [--build] [-b|--builder] [--builder-image] [-c|--confirm]
	             [-v|--verbose]

DESCRIPTION
	Run the function locally.

	Values provided for flags are not persisted to the function's metadata.

	Containerized Runs
	  The --container flag indicates that the function's container should be
	  run rather than running the source code directly.  This may require that
	  the function's container first be rebuilt.  Building the container on or
	  off can be altered using the --build flag.  The default value --build=auto
	  indicates the system should automatically build the container only if
	  necessary.

	Process Scaffolding
	  This is an Experimental Feature currently available only to Go projects.
	  When running a function with --container=false (host-based runs), the
	  function is first wrapped code which presents it as a process.
	  This "scaffolding" is transient, written for each build or run, and should
	  in most cases be transparent to a function author.  However, to customize,
	  or even completely replace this scafolding code, see the 'scaffold'
	  subcommand.

EXAMPLES

	o Run the function locally from within its container.
	  $ func run

	o Run the function locally from within its container, forcing a rebuild
	  of the container even if no filesysem changes are detected
	  $ func run --build

	o Run the function locally on the host with no containerization (Go only).
	  $ func run --container=false


```
func run
```

### Options

```
      --build string[="true"]   Build the function. [auto|true|false]. ($FUNC_BUILD) (default "auto")
  -b, --builder string          Builder to use when creating the function's container. Currently supported builders are "pack" and "s2i". (default "pack")
      --builder-image string    Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)
  -c, --confirm                 Prompt to confirm options interactively ($FUNC_CONFIRM)
  -t, --container               Run the function in a container. ($FUNC_CONTAINER) (default true)
  -e, --env stringArray         Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables. To unset, specify the environment variable name followed by a "-" (e.g., NAME-).
  -h, --help                    help for run
  -i, --image string            Full image name in the form [registry]/[namespace]/[name]:[tag]. This option takes precedence over --registry. Specifying tag is optional. ($FUNC_IMAGE)
  -p, --path string             Path to the function.  Default is current directory ($FUNC_PATH)
  -r, --registry string         Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)
  -v, --verbose                 Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

