## func version

Show the version

### Synopsis


NAME
	func version - function version information.

SYNOPSIS
	func version [-v|--verbose]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.

	o Print the functions version
	  $ func version

	o Print the functions version along with date and associated git commit hash.
	  $ func version -v



```
func version
```

### Options

```
  -h, --help   help for version
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

