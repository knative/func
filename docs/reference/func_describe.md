## func describe

Describe a Function

### Synopsis

Describe a Function

Prints the name, route and event subscriptions for a deployed function in
the current directory or from the directory specified with --path.


```
func describe <name>
```

### Examples

```

# Show the details of a function as declared in the local func.yaml
func info

# Show the details of the function in the directory with yaml output
func info --output yaml --path myotherfunc

```

### Options

```
  -h, --help               help for describe
  -n, --namespace string   The namespace in which to look for the named function. (Env: $FUNC_NAMESPACE) (default "default")
  -o, --output string      Output format (human|plain|json|xml|yaml|url) (Env: $FUNC_OUTPUT) (default "human")
  -p, --path string        Path to the project directory.  Default is current working directory (Env: $FUNC_PATH)
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

