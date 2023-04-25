## func config envs

List and manage configured environment variable for a function

### Synopsis

List and manage configured environment variable for a function

Prints configured Environment variable for a function project present in
the current directory or from the directory specified with --path.


```
func config envs
```

### Options

```
  -h, --help            help for envs
  -o, --output string   Output format (human|json) ($FUNC_OUTPUT) (default "human")
  -p, --path string     Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config](func_config.md)	 - Configure a function
* [func config envs add](func_config_envs_add.md)	 - Add environment variable to the function configuration
* [func config envs remove](func_config_envs_remove.md)	 - Remove environment variable from the function configuration

