## func delete

Undeploy a function

### Synopsis

Undeploy a function

This command undeploys a function from the cluster. By default the function from
the project in the current directory is undeployed. Alternatively either the name
of the function can be given as argument or the project path provided with --path.

No local files are deleted.


```
func delete [NAME]
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
  -a, --all string         Delete all resources created for a function, eg. Pipelines, Secrets, etc. (Env: $FUNC_ALL) (allowed values: "true", "false") (default "true")
  -c, --confirm            Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)
  -h, --help               help for delete
  -n, --namespace string   The namespace in which to delete. (Env: $FUNC_NAMESPACE)
  -p, --path string        Path to the project directory (Env: $FUNC_PATH) (default ".")
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

