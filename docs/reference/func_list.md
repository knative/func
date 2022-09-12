## func list

List functions

### Synopsis

List functions

Lists all deployed functions in a given namespace.


```
func list
```

### Examples

```

# List all functions in the current namespace with human readable output
func list

# List all functions in the 'test' namespace with yaml output
func list --namespace test --output yaml

# List all functions in all namespaces with JSON output
func list --all-namespaces --output json

```

### Options

```
  -A, --all-namespaces   List functions in all namespaces. If set, the --namespace flag is ignored.
  -h, --help             help for list
  -o, --output string    Output format (human|plain|json|xml|yaml) (Env: $FUNC_OUTPUT) (default "human")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

