## func envs

Manage function environment variables

### Synopsis

func envs

Manages function environment variables.  Default is to list currently configured
environment variables for the function.  See subcommands 'add' and 'remove'.

```
func envs
```

### Options

```
  -h, --help            help for envs
  -o, --output string   Output format (human|json) (Env: $FUNC_OUTPUT) (default "human")
  -p, --path string     Path to the function.  Default is current directory (Env: $FUNC_PATH)
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func envs add](func_envs_add.md)	 - Add environment variable to the function
* [func envs remove](func_envs_remove.md)	 - Remove environment variable from the function configuration

