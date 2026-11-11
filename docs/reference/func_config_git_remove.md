## func config git remove

Remove Git settings from the function configuration

### Synopsis

Remove Git settings from the function configuration

	Interactive prompt to remove Git settings from the function project in the current
	directory or from the directory specified with --path.

	It also removes any generated resources that are used for Git based build and deployment,
	such as local generated Pipelines resources and any resources generated on the cluster.


```
func config git remove
```

### Options

```
      --delete-cluster[=true|false]   Delete cluster resources (credentials and config on the cluster).
      --delete-local[=true|false]     Delete local resources (pipeline templates).
  -h, --help[=true|false]             help for remove
  -p, --path string                   Path to the function.  Default is current directory ($FUNC_PATH)
  -v, --verbose[=true|false]          Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func config git](func_config_git.md)	 - Manage Git configuration of a function

