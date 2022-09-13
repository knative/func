## func deploy

Deploy a Function

### Synopsis


NAME
	func deploy - Deploy a Function

SYNOPSIS
	func deploy [-R|--remote] [-r|--registry] [-i|--image] [-n|--namespace]
	             [-e|env] [-g|--git-url] [-t|git-branch] [-d|--git-dir]
	             [-b|--build] [--builder] [--builder-image] [-p|--push]
	             [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Deploys a function to the currently configured Knative-enabled cluster.

	By default the function in the current working directory is deployed, or at
	the path defined by --path.

	A function which was previously deployed will be updated when re-deployed.

	The function is built into a container for transport to the destination
	cluster by way of a registry.  Therefore --registry must be provided or have
	previously been configured for the function. This registry is also used to
	determine the final built image tag for the function.  This final image name
	can be provided explicitly using --image, in which case it is used in place
	of --registry.

	To run deploy using an interactive mode, use the --confirm (-c) option.
	This mode is useful for the first deployment in particular, since subsdequent
	deployments remember most of the settings provided.

	Building
	  By default the function will be built if it has not yet been built, or if
	  changes are detected in the function's source.  The --build flag can be
	  used to override this behavior and force building either on or off.

	Remote
	  Building and pushing (deploying) is by default run on localhost.  This
	  process can also be triggered to run remotely in a Tekton-enabled cluster.
	  The --remote flag indicates that a build and deploy pipeline should be
	  invoked in the remote.  Functions deployed in this manner must have their
	  source code kept in a git repository, and the URL to this source provided
	  via --git-url.  A specific branch can be specified with --git-branch.

EXAMPLES

	o Deploy the function using interactive prompts. This is useful for the first
	  deployment, since most settings will be remembered for future deployments.
	  $ func deploy -c

	o Deploy the function in the current working directory.
	  The function image will be pushed to "ghcr.io/alice/<Function Name>"
	  $ func deploy --registry ghcr.io/alice

	o Deploy the function in the current working directory, manually specifying
	  the final image name and target cluster namespace.
	  $ func deploy --image ghcr.io/alice/myfunc --namespace myns

	o Trigger a remote deploy, which instructs the cluster to build and deploy
	  the function in the specified git repository.
	  $ func deploy --remote --git-url=https://example.com/alice/myfunc.git

	o Deploy the function, rebuilding the image even if no changes have been
	  detected in the local filesystem (source).
	  $ func deploy --build

	o Deploy without rebuilding, even if changes have been detected in the
	  local filesystem.
	  $ func deploy --build=false



```
func deploy [flags]
```

### Options

```
      --build string[="true"]   Build the function. [auto|true|false]. [Env: $FUNC_BUILD] (default "auto")
  -b, --builder string          builder to use when creating the underlying image. Currently supported builders are "pack" and "s2i". (default "pack")
      --builder-image string    The image the specified builder should use; either an as an image name or a mapping. ($FUNC_BUILDER_IMAGE)
  -c, --confirm                 Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)
  -e, --env stringArray         Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables. To unset, specify the environment variable name followed by a "-" (e.g., NAME-).
  -t, --git-branch string       Git branch to be used for remote builds (Env: $FUNC_GIT_BRANCH)
  -d, --git-dir string          Directory in the repo where the function is located (Env: $FUNC_GIT_DIR)
  -g, --git-url string          Repo url to push the code to be built (Env: $FUNC_GIT_URL)
  -h, --help                    help for deploy
  -i, --image string            Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)
  -p, --path string             Path to the project directory (Env: $FUNC_PATH) (default ".")
      --platform string         Target platform to build (e.g. linux/amd64).
  -u, --push                    Push the function image to registry before deploying (Env: $FUNC_PUSH) (default true)
  -r, --registry string         Registry + namespace part of the image to build, ex 'ghcr.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)
      --remote                  Trigger a remote deployment.  Default is to deploy and build from the local system: $FUNC_REMOTE)
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

