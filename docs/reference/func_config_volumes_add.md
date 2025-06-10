## func config volumes add

Add volume to the function configuration

### Synopsis

Add volume to the function configuration

Interactive prompt to add Secrets and ConfigMaps as Volume mounts to the function project
in the current directory or from the directory specified with --path.

For non-interactive usage, use flags to specify the volume type and configuration.


```
func config volumes add
```

### Examples

```
# Add a ConfigMap volume
func config volumes add --type=configmap --source=my-config --path=/etc/config

# Add a Secret volume
func config volumes add --type=secret --source=my-secret --path=/etc/secret

# Add a PersistentVolumeClaim volume
func config volumes add --type=pvc --source=my-pvc --path=/data
func config volumes add --type=pvc --source=my-pvc --path=/data --read-only

# Add an EmptyDir volume
func config volumes add --type=emptydir --path=/tmp/cache
func config volumes add --type=emptydir --path=/tmp/cache --size=1Gi --medium=Memory
```

### Options

```
  -h, --help                help for add
      --medium string       Storage medium for EmptyDir volume: 'Memory' or '' (default)
  -m, --mount-path string   Path where the volume should be mounted in the container
  -p, --path string         Path to the function.  Default is current directory ($FUNC_PATH)
  -r, --read-only           Mount volume as read-only (only for PVC)
      --size string         Maximum size limit for EmptyDir volume (e.g., 1Gi)
  -s, --source string       Name of the ConfigMap, Secret, or PVC to mount (not used for emptydir)
  -t, --type string         Volume type: configmap, secret, pvc, or emptydir
  -v, --verbose             Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config volumes](func_config_volumes.md)	 - List and manage configured volumes for a function

