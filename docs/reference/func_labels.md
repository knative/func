## func labels

Manage function labels

### Synopsis

func labels

Manages function labels.  Default is to list currently configured labels.  See
subcommands 'add' and 'remove'.

```
func labels
```

### Options

```
  -h, --help            help for labels
  -o, --output string   Output format (human|json) (Env: $FUNC_OUTPUT) (default "human")
  -p, --path string     Path to the function.  Default is current directory (Env: $FUNC_PATH)
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func labels add](func_labels_add.md)	 - Add label to the function
* [func labels remove](func_labels_remove.md)	 - Remove labels from the function configuration

