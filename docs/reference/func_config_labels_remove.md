## func config labels remove

Remove labels from the function configuration

### Synopsis

Remove labels from the function configuration

Interactive prompt to remove labels from the function project in the current
directory or from the directory specified with --path.


```
func config labels remove
```

### Options

```
  -h, --help          help for remove
  -p, --path string   Path to the project directory (Env: $FUNC_PATH) (default ".")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config labels](func_config_labels.md)	 - List and manage configured labels for a function

