## func logs

Stream logs from a deployed function

### Synopsis

Stream logs from a deployed function

Streams logs for the function in the current directory or from the directory
specified with --path. Abstracts away the underlying service name and pod details.


```
func logs
```

### Examples

```

# Stream logs for the function in the current directory
func logs

# Stream logs for a function by name
func logs --name my-function

# Stream logs from a specific namespace
func logs --namespace my-namespace

# Stream logs with a specific time window
func logs --since 5m

```

### Options

```
  -h, --help               help for logs
      --name string        Name of the function to get logs from ($FUNC_NAME)
  -n, --namespace string   The namespace of the function ($FUNC_NAMESPACE) (default "default")
  -p, --path string        Path to the function.  Default is current directory ($FUNC_PATH)
      --since string       Return logs newer than a relative duration like 5s, 2m, or 3h ($FUNC_LOGS_SINCE) (default "1m")
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### Options inherited from parent commands

```
      --json   Output results as JSON ($FUNC_JSON)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

