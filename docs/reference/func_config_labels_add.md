## func config labels add

Add labels to the function configuration

### Synopsis

Add labels to the function configuration

If label is not set explicitly by flag, interactive prompt is used.

The label can be set directly from a value or from an environment variable on
the local machine.


```
func config labels add
```

### Examples

```
# set label directly
func config labels add --name=Foo --value=Bar

# set label from local env $FOO
func config labels add --name=Foo --value='{{ env:FOO }}'
```

### Options

```
  -h, --help           help for add
      --name string    Name of the label.
  -p, --path string    Path to the function.  Default is current directory ($FUNC_PATH)
      --value string   Value of the label.
  -v, --verbose        Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config labels](func_config_labels.md)	 - List and manage configured labels for a function

