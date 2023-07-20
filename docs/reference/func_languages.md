## func languages

List available function language runtimes

### Synopsis


NAME
	func languages - list available language runtimes.

SYNOPSIS
	func languages [--json] [-r|--repository]

DESCRIPTION
	List the language runtimes that are currently available.
	This includes embedded (included) language runtimes as well as any installed
	via the 'repositories add' command.

	To specify a URI of a single, specific repository for which languages
	should be displayed, use the --repository flag.

	Installed repositories are by default located at ~/.func/repositories
	($XDG_CONFIG_HOME/.func/repositories).  This can be overridden with
	$FUNC_REPOSITORIES_PATH.

	To see templates available for a given language, see the 'templates' command.


EXAMPLES

	o Show a list of all available language runtimes
	  $ func languages

	o Return a list of all language runtimes in JSON
	  $ func languages --json

	o Return language runtimes in a specific repository
		$ func languages --repository=https://github.com/boson-project/templates


```
func languages
```

### Options

```
  -h, --help                help for languages
      --json                Set output to JSON format. ($FUNC_JSON)
  -r, --repository string   URI to a specific repository to consider ($FUNC_REPOSITORY)
  -v, --verbose             Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

