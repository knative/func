## func deploy

Deploy a function

### Synopsis

Deploy a function

Builds a container image for the function and deploys it to the connected Knative enabled cluster. 
The function is picked up from the project in the current directory or from the path provided
with --path.
If not already configured, either --registry or --image has to be provided and is then stored 
in the configuration file.

If the function is already deployed, it is updated with a new container image
that is pushed to an image registry, and finally the function's Knative service is updated.


```
func deploy [flags]
```

### Examples

```

# Build and deploy the function from the current directory's project. The image will be
# pushed to "quay.io/myuser/<function name>" and deployed as Knative service with the 
# same name as the function to the currently connected cluster.
func deploy --registry quay.io/myuser

# Same as above but using a full image name, that will create a Knative service "myfunc" in 
# the namespace "myns"
func deploy --image quay.io/myuser/myfunc -n myns

```

### Options

```
  -b, --build string           Build specifies the way the function should be built. Supported types are "disabled", "local" or "git" (Env: $FUNC_BUILD) (default "local")
      --builder string         build strategy to use when creating the underlying image. Currently supported build strategies are "pack" or "s2i". (default "pack")
      --builder-image string   builder image, either an as a an image name or a mapping name.
                               Specified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)
  -c, --confirm                Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)
  -e, --env stringArray        Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables. To unset, specify the environment variable name followed by a "-" (e.g., NAME-).
  -t, --git-branch string      Git branch to be used for remote builds (Env: $FUNC_GIT_BRANCH)
  -d, --git-dir string         Directory in the repo where the function is located (Env: $FUNC_GIT_DIR)
  -g, --git-url string         Repo url to push the code to be built (Env: $FUNC_GIT_URL)
  -h, --help                   help for deploy
  -i, --image string           Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)
  -p, --path string            Path to the project directory (Env: $FUNC_PATH) (default "/Users/lball/src/github.com/knative-sandbox/kn-plugin-func")
      --platform string        Target platform to build (e.g. linux/amd64).
  -u, --push                   Attempt to push the function image to registry before deploying (Env: $FUNC_PUSH) (default true)
  -r, --registry string        Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

