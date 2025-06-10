## func config volumes remove

Remove volume from the function configuration

### Synopsis

Remove volume from the function configuration

Interactive prompt to remove Volume mounts from the function project
in the current directory or from the directory specified with --path.

For non-interactive usage, use the --mount-path flag to specify which volume to remove.


```
func config volumes remove
```

### Examples

```
# Remove a volume by its mount path
func config volumes remove --mount-path=/etc/config
```

### Options

```
  -h, --help                help for remove
  -m, --mount-path string   Path of the volume mount to remove
  -p, --path string         Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose             Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config volumes](func_config_volumes.md)	 - List and manage configured volumes for a function

