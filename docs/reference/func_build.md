## func build

Build a function project as a container image

### Synopsis

Build a function project as a container image

This command builds the function project in the current directory or in the directory
specified by --path. The result will be a container image that is pushed to a registry.
The func.yaml file is read to determine the image name and registry.
If the project has not already been built, either --registry or --image must be provided
and the image name is stored in the configuration file.


```
func build
```

### Examples

```

# Build from the local directory, using the given registry as target.
# The full image name will be determined automatically based on the
# project directory name
func build --registry quay.io/myuser

# Build from the local directory, specifying the full image name
func build --image quay.io/myuser/myfunc

# Re-build, picking up a previously supplied image name from a local func.yml
func build

# Build using s2i instead of Buildpacks
func build --builder=s2i

# Build with a custom buildpack builder
func build --builder=pack --builder-image cnbs/sample-builder:bionic

```

### Options

```
  -b, --builder string         build strategy to use when creating the underlying image. Currently supported build strategies are "pack" and "s2i". (default "pack")
      --builder-image string   builder image, either an as a an image name or a mapping name.
                               Specified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)
  -c, --confirm                Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)
  -h, --help                   help for build
  -i, --image string           Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE)
  -p, --path string            Path to the project directory.  Default is current working directory (Env: $FUNC_PATH)
      --platform string        Target platform to build (e.g. linux/amd64).
  -u, --push                   Attempt to push the function image after being successfully built
  -r, --registry string        Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined (Env: $FUNC_REGISTRY) (default "image-registry.openshift-image-registry.svc:5000/default")
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

