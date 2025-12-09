## func version

Function client version information

### Synopsis


NAME
	func version - function version information.

SYNOPSIS
	func version [-v|--verbose] [-o|--output]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.  Use the --output option
	to specify the output format (human|json|yaml).

	o Print the functions version
	  $ func version

	o Print the functions version along with source git commit hash and other
	  metadata.
	  $ func version -v

	o Print the version information in JSON format
	  $ func version --output json

	o Print verbose version information in YAML format
	  $ func version -v -o yaml



```
func version
```

### Options

```
  -h, --help            help for version
  -o, --output string   Output format (human|json|yaml) ($FUNC_OUTPUT) (default "human")
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

