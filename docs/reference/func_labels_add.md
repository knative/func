## func labels add

Add label to the function

### Synopsis

Add labels to the function

Add labels to the function project in the current directory or from the
directory specified with --path.  If no flags are provided, addition is
completed using interactive prompts.

The label can be set directly from a value or from an environment variable on
the local machine.

```
func labels add
```

### Examples

```
# add a label
func labels add --key=myLabel --value=myValue

# add a label from a local environment variable
func labels add --key=myLabel --value='{{ env:LOC_ENV }}'
```

### Options

```
  -h, --help           help for add
      --key string     Key of the label.
  -p, --path string    Path to the function.  Default is current directory (Env: $FUNC_PATH)
      --value string   Value of the label.
  -v, --verbose        Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func labels](func_labels.md)	 - Manage function labels

