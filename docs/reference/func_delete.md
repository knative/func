## func delete

Undeploy a function

### Synopsis

Undeploy a function

This command undeploys a function from the cluster. By default the function from
the project in the current directory is undeployed. Alternatively either the name
of the function can be given as argument or the project path provided with --path.

No local files are deleted.


```
func delete <name>
```

### Examples

```

# Undeploy the function defined in the local directory
func delete

# Undeploy the function 'myfunc' in namespace 'apps'
func delete -n apps myfunc

```

### Options

```
  -a, --all string         Delete all resources created for a function, eg. Pipelines, Secrets, etc. ($FUNC_ALL) (allowed values: "true", "false") (default "true")
  -c, --confirm            Prompt to confirm options interactively ($FUNC_CONFIRM)
  -h, --help               help for delete
  -n, --namespace string   The namespace in which to delete. ($FUNC_NAMESPACE)
  -p, --path string        Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

