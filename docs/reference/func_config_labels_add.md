## func config labels add

Add labels to the function configuration

### Synopsis

Add labels to the function configuration

Interactive prompt to add labels to the function project in the current
directory or from the directory specified with --path.

The label can be set directly from a value or from an environment variable on
the local machine.


```
func config labels add
```

### Options

```
  -h, --help          help for add
  -p, --path string   Path to the project directory (Env: $FUNC_PATH) (default ".")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config labels](func_config_labels.md)	 - List and manage configured labels for a function

