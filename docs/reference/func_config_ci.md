## func config ci

Generate a GitHub Workflow for function deployment

```
func config ci
```

### Options

```
      --branch string                             Use a custom branch name in the workflow (default "main")
  -h, --help                                      help for ci
      --kubeconfig-secret-name string             Use a custom secret name in the workflow, e.g. secret.YOUR_CUSTOM_KUBECONFIG (default "KUBECONFIG")
  -p, --path string                               Path to the function.  Default is current directory ($FUNC_PATH)
      --registry-login-url-variable-name string   Use a custom registry login url variable name in the workflow, e.g. vars.YOUR_REGISTRY_LOGIN_URL (default "REGISTRY_LOGIN_URL")
      --registry-pass-secret-name string          Use a custom registry pass secret name in the workflow, e.g. secret.YOUR_REGISTRY_PASSWORD (default "REGISTRY_PASSWORD")
      --registry-url-variable-name string         Use a custom registry url variable name in the workflow, e.g. vars.YOUR_REGISTRY_URL (default "REGISTRY_URL")
      --registry-user-variable-name string        Use a custom registry user variable name in the workflow, e.g. vars.YOUR_REGISTRY_USER (default "REGISTRY_USERNAME")
      --remote                                    Build the function on a Tekton-enabled cluster
      --self-hosted-runner                        Use a 'self-hosted' runner instead of the default 'ubuntu-latest' for local runner execution
      --use-registry-login                        Add a registry login step in the github workflow (default true)
      --workflow-name string                      Use a custom workflow name (default "Func Deploy")
```

### SEE ALSO

* [func config](func_config.md)	 - Configure a function

