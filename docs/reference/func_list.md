## func list

List deployed functions

### Synopsis

List deployed functions

Lists deployed functions.


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
  -n, --namespace string   The namespace for which to list functions. ($FUNC_NAMESPACE) (default "default")
  -o, --output string      Output format (human|plain|json|xml|yaml) ($FUNC_OUTPUT) (default "human")
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

