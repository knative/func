## func volumes add

Add volume to the function

### Synopsis

Add volume to the function

Add secrets and config maps as volume mounts to the function in the current
directory or from the directory specified by --path.  If no flags are provided,
addition is completed using interactive prompts.

The volume can be set


```
func volumes add
```

### Options

```
      --configmap string   Name of the config map to mount.
  -h, --help               help for add
      --mount string       Mount path.
  -p, --path string        Path to the function.  Default is current directory (Env: $FUNC_PATH)
      --secret string      Name of the secret to mount.
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func volumes](func_volumes.md)	 - Manage function volumes

