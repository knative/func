## func logs

Print or stream logs from a deployed function

### Synopsis

Print or stream logs from a deployed function

Prints the logs of the function in the current directory or of the directory
specified with --path and exits. Use --follow to stream logs until interrupted.
Abstracts away the underlying service name and pod details.

When more than one pod is serving the function, each line is prefixed with the
pod it came from. A function which has scaled to zero has no pods, and thus no
logs to print.

Only functions deployed with the default Knative deployer are currently
supported.


```
func logs
```

### Examples

```

# Print the logs of the function in the current directory and exit
func logs

# Stream logs until interrupted
func logs -f

# Print logs for a function by name
func logs --name my-function

# Print logs from a specific namespace
func logs --namespace my-namespace

# Print logs of a specific time window
func logs --since 5m

# Print the last 20 log lines per pod
func logs --tail 20

# Stream logs, starting with those of the last 5 minutes
func logs -f --since 5m

```

### Options

```
  -f, --follow             Stream logs until interrupted rather than printing and exiting ($FUNC_FOLLOW)
  -h, --help               help for logs
      --name string        Name of the function to get logs from ($FUNC_NAME)
  -n, --namespace string   The namespace of the function ($FUNC_NAMESPACE) (default "default")
  -p, --path string        Path to the function.  Default is current directory ($FUNC_PATH)
      --since string       Return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all available logs, or to the last minute when following without --tail ($FUNC_SINCE)
      --tail int           Number of most recent log lines to return per pod. Unlimited if negative ($FUNC_TAIL) (default -1)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

