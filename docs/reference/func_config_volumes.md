## func config volumes

List and manage configured volumes for a function

### Synopsis

List and manage configured volumes for a function

Prints configured Volume mounts for a function project present in
the current directory or from the directory specified with --path.


```
func config volumes [flags]
```

### Options

```
  -h, --help          help for volumes
  -p, --path string   Path to the project directory (Env: $FUNC_PATH) (default "/Users/lball/src/github.com/knative-sandbox/kn-plugin-func")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config](func_config.md)	 - Configure a function
* [func config volumes add](func_config_volumes_add.md)	 - Add volume to the function configuration
* [func config volumes remove](func_config_volumes_remove.md)	 - Remove volume from the function configuration

