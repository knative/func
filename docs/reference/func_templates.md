## func templates

List available function source templates

### Synopsis


NAME
	func templates - list available function source templates

SYNOPSIS
	func templates [language] [--json] [-r|--repository]

DESCRIPTION
	List all templates available, optionally for a specific language runtime.

	To specify a URI of a single, specific repository for which templates
	should be displayed, use the --repository flag.

	Installed repositories are by default located at ~/.func/repositories
	($XDG_CONFIG_HOME/.func/repositories).  This can be overridden with
	$FUNC_REPOSITORIES_PATH.

	To see all available language runtimes, see the 'languages' command.


EXAMPLES

	o Show a list of all available templates grouped by language runtime
	  $ func templates

	o Show a list of all templates for the Go runtime
	  $ func templates go

	o Return a list of all template runtimes in JSON output format
	  $ func templates --json

	o Return Go templates in a specific repository
		$ func templates go --repository=https://github.com/boson-project/templates


```
func templates
```

### Options

```
  -h, --help                help for templates
      --json                Set output to JSON format. (Env: $FUNC_JSON)
  -r, --repository string   URI to a specific repository to consider ($FUNC_REPOSITORY)
  -v, --verbose             Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

