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
  -A, --all-namespaces     List functions in all namespaces. If set, the --namespace flag is ignored.
  -h, --help               help for list
  -n, --namespace string   The namespace for which to list functions. (Env: $FUNC_NAMESPACE)
  -o, --output string      Output format (human|plain|json|xml|yaml) (Env: $FUNC_OUTPUT) (default "human")
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

