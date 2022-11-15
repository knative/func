## func

Serverless functions

### Synopsis

Knative serverless functions

	Create, build and deploy Knative functions

SYNOPSIS
	func [-v|--verbose] <command> [args]

EXAMPLES

	o Create a Node function in the current directory
	  $ func create --language node .

	o Deploy the function defined in the current working directory to the
	  currently connected cluster, specifying a container registry in place of
	  quay.io/user for the function's container.
	  $ func deploy --registry quay.io.user

	o Invoke the function defined in the current working directory with an example
	  request.
	  $ func invoke

	For more examples, see 'func [command] --help'.

### Options

```
  -h, --help      help for func
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func build](func_build.md)	 - Build a Function
* [func completion](func_completion.md)	 - Generate completion scripts for bash, fish and zsh
* [func config](func_config.md)	 - Configure a function
* [func create](func_create.md)	 - Create a function project
* [func delete](func_delete.md)	 - Undeploy a function
* [func deploy](func_deploy.md)	 - Deploy a Function
* [func describe](func_describe.md)	 - Describe a Function
* [func invoke](func_invoke.md)	 - Invoke a function
* [func languages](func_languages.md)	 - List available function language runtimes
* [func list](func_list.md)	 - List functions
* [func repository](func_repository.md)	 - Manage installed template repositories
* [func run](func_run.md)	 - Run the function locally
* [func templates](func_templates.md)	 - Templates
* [func version](func_version.md)	 - Show the version

