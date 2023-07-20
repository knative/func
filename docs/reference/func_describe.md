## func describe

Describe a function

### Synopsis

Describe a function

Prints the name, route and event subscriptions for a deployed function in
the current directory or from the directory specified with --path.


```
func describe <name>
```

### Examples

```

# Show the details of a function as declared in the local func.yaml
func describe

# Show the details of the function in the directory with yaml output
func describe --output yaml --path myotherfunc

```

### Options

```
  -h, --help               help for describe
  -n, --namespace string   The namespace in which to look for the named function. ($FUNC_NAMESPACE)
  -o, --output string      Output format (human|plain|json|xml|yaml|url) ($FUNC_OUTPUT) (default "human")
  -p, --path string        Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

