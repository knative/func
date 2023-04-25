## func config

Configure a function

### Synopsis

Configure a function

Interactive prompt that allows configuration of Git configuration, Volume mounts, Environment
variables, and Labels for a function project present in the current directory
or from the directory specified with --path.


```
func config
```

### Options

```
  -h, --help          help for config
  -p, --path string   Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose       Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func config envs](func_config_envs.md)	 - List and manage configured environment variable for a function
* [func config git](func_config_git.md)	 - Manage Git configuration of a function
* [func config labels](func_config_labels.md)	 - List and manage configured labels for a function
* [func config volumes](func_config_volumes.md)	 - List and manage configured volumes for a function

