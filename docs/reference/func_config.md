## func config

Configure a function

### Synopsis

Configure a function

Interactive prompt that allows configuration of Volume mounts, Environment
variables, and Labels for a function project present in the current directory
or from the directory specified with --path.


```
func config
```

### Options

```
  -h, --help          help for config
  -p, --path string   Path to the project directory.  Default is current working directory (Env: $FUNC_PATH)
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions
* [func config envs](func_config_envs.md)	 - List and manage configured environment variable for a function
* [func config labels](func_config_labels.md)	 - List and manage configured labels for a function
* [func config volumes](func_config_volumes.md)	 - List and manage configured volumes for a function

