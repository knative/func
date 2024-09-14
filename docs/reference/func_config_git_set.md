## func config git set

Set Git settings in the function configuration

### Synopsis

Set Git settings in the function configuration

	Interactive prompt to set Git settings in the function project in the current
	directory or from the directory specified with --path.


```
func config git set
```

### Options

```
  -b, --builder string             Builder to use when creating the function's container. Currently supported builders are "pack" and "s2i". (default "s2i")
      --builder-image string       Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)
      --config-cluster             Configure cluster resources (credentials and config on the cluster).
      --config-local               Configure local resources (pipeline templates).
      --config-remote              Configure remote resources (webhook on the Git provider side).
      --gh-access-token string     GitHub Personal Access Token. For public repositories the scope is 'public_repo', for private is 'repo'. If you want to configure the webhook automatically, 'admin:repo_hook' is needed as well. Get more details: https://pipelines-as-code.pages.dev/docs/install/github_webhook/.
      --gh-webhook-secret string   GitHub Webhook Secret used for payload validation. If not specified, it will be generated automatically.
  -t, --git-branch string          Git revision (branch) to be used when deploying via the Git repository ($FUNC_GIT_BRANCH)
  -d, --git-dir string             Directory in the Git repository containing the function (default is the root) ($FUNC_GIT_DIR)
      --git-provider string        The type of the Git platform provider to setup webhook. This value is usually automatically generated from input URL, use this parameter to override this setting. Currently supported providers are "github" and "gitlab".
  -g, --git-url string             Repository url containing the function to build ($FUNC_GIT_URL)
  -h, --help                       help for set
  -i, --image string               Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. ($FUNC_IMAGE)
  -n, --namespace string           Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. ($FUNC_NAMESPACE)
  -p, --path string                Path to the function.  Default is current directory ($FUNC_PATH)
  -r, --registry string            Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)
  -v, --verbose                    Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config git](func_config_git.md)	 - Manage Git configuration of a function

