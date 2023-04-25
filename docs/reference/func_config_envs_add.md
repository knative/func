## func config envs add

Add environment variable to the function configuration

### Synopsis

Add environment variable to the function configuration.

If environment variable is not set explicitly by flag, interactive prompt is used.

The environment variable can be set directly from a value,
from an environment variable on the local machine or from Secrets and ConfigMaps.
It is also possible to import all keys as environment variables from a Secret or ConfigMap.

```
func config envs add
```

### Examples

```
# set environment variable directly
func config envs add --name=VARNAME --value=myValue

# set environment variable from local env $LOC_ENV
func config envs add --name=VARNAME --value='{{ env:LOC_ENV }}'

set environment variable from a secret
func config envs add --name=VARNAME --value='{{ secret:secretName:key }}'

# set all key as environment variables from a secret
func config envs add --value='{{ secret:secretName }}'

# set environment variable from a configMap
func config envs add --name=VARNAME --value='{{ configMap:confMapName:key }}'

# set all key as environment variables from a configMap
func config envs add --value='{{ configMap:confMapName }}'
```

### Options

```
  -h, --help           help for add
      --name string    Name of the environment variable.
  -p, --path string    Path to the function.  Default is current directory ($FUNC_PATH)
      --value string   Value of the environment variable.
  -v, --verbose        Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config envs](func_config_envs.md)	 - List and manage configured environment variable for a function

