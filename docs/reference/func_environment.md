## func environment

Display function execution environment information

### Synopsis


NAME
	func environment - display function execution environment information

SYNOPSIS
	func environment [-f|--format] [-v|--verbose] [-p|--path]


DESCRIPTION
	Display information about the function execution environment, including
	the version of func, the version of the function spec, the default builder,
	available runtimes, and available templates.


```
func environment
```

### Options

```
  -f, --format string   Format of output environment information, 'json' or 'yaml'. ($FUNC_FORMAT) (default "json")
  -h, --help            help for environment
  -p, --path string     Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

