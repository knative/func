## func volumes

Manage function volumes

### Synopsis

func volumes

Manges function volumes mounts.  Default is to list currently configured
volumes.  See subcommands 'add' and 'remove'.


```
func volumes
```

### Options

```
  -h, --help            help for volumes
  -o, --output string   Output format (human|json) (Env: $FUNC_OUTPUT) (default "human")
  -p, --path string     Path to the function.  Default is current directory (Env: $FUNC_PATH)
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func volumes add](func_volumes_add.md)	 - Add volume to the function
* [func volumes remove](func_volumes_remove.md)	 - Remove volume from the function configuration

