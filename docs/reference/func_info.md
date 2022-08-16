## func info

Show details of a function

### Synopsis

Show details of a function

Prints the name, route and any event subscriptions for a deployed function in
the current directory or from the directory specified with --path.


```
func info <name> [flags]
```

### Examples

```

# Show the details of a function as declared in the local func.yaml
func info

# Show the details of the function in the myotherfunc directory with yaml output
func info --output yaml --path myotherfunc

```

### Options

```
  -h, --help            help for info
  -o, --output string   Output format (human|plain|json|xml|yaml|url) (Env: $FUNC_OUTPUT) (default "human")
  -p, --path string     Path to the project directory (Env: $FUNC_PATH) (default "/Users/lball/src/github.com/knative-sandbox/kn-plugin-func")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

