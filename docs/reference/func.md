## func

func manages Knative Functions

### Synopsis

func is the command line interface for managing Knative Function resources

	Create a new Node.js function in the current directory:
	func create --language node myfunction

	Deploy the function using Docker hub to host the image:
	func deploy --registry docker.io/alice

Learn more about Functions:  https://knative.dev/docs/functions/
Learn more about Knative at: https://knative.dev

### Options

```
  -h, --help   help for func
```

### SEE ALSO

* [func build](func_build.md)	 - Build a function container
* [func completion](func_completion.md)	 - Output functions shell completion code
* [func config](func_config.md)	 - Configure a function
* [func create](func_create.md)	 - Create a function
* [func delete](func_delete.md)	 - Undeploy a function
* [func deploy](func_deploy.md)	 - Deploy a function
* [func describe](func_describe.md)	 - Describe a function
* [func environment](func_environment.md)	 - Display function execution environment information
* [func invoke](func_invoke.md)	 - Invoke a local or remote function
* [func languages](func_languages.md)	 - List available function language runtimes
* [func list](func_list.md)	 - List deployed functions
* [func repository](func_repository.md)	 - Manage installed template repositories
* [func run](func_run.md)	 - Run the function locally
* [func subscribe](func_subscribe.md)	 - Subscribe a function to events
* [func templates](func_templates.md)	 - List available function source templates
* [func version](func_version.md)	 - Function client version information

