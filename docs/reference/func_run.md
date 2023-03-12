## func run

Run the function locally

### Synopsis

Run the function locally

Runs the function locally in the current directory or in the directory
specified by --path flag.

Building
By default the function will be built if never built, or if changes are detected
to the function's source.  Use --build to override this behavior.
Also a builder strategy (pack, s2i) can be chosen using the --builder option.
Default builder is pack.



```
func run
```

### Examples

```

# Run the function locally, building if necessary
func run

# Run the function locally, building if necessary, with --builder option
func run --builder s2i

# Run the function, forcing a rebuild of the image.
#   This is useful when the function's image was manually deleted, necessitating
#   A rebuild even when no changes have been made the function's source.
func run --build

# Run the function, forcing a rebuild of the image with --builder option.
func run --build --builder s2i

# Run the function's existing image, disabling auto-build.
#   This is useful when filesystem changes have been made, but one wishes to
#   run the previously built image without rebuilding.
func run --build=false


```

### Options

```
  -b, --build string[="true"]   Build the function. [auto|true|false]. (default "auto")
      --builder string          Builder to use when creating the function's container. Currently supported builders are "pack" and "s2i". (Env: $FUNC_BUILDER) (default "pack")
  -e, --env stringArray         Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables. To unset, specify the environment variable name followed by a "-" (e.g., NAME-).
  -h, --help                    help for run
  -p, --path string             Path to the function.  Default is current directory (Env: $FUNC_PATH)
  -r, --registry string         Registry + namespace part of the image if building, ex 'quay.io/myuser' (Env: $FUNC_REGISTRY)
  -v, --verbose                 Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

