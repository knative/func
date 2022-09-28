## func create

Create a function project

### Synopsis


NAME
	func create - Create a function project.

SYNOPSIS
	func create [-l|--language] [-t|--template] [-r|--repository]
	            [-c|--confirm]  [-v|--verbose]  [path]

DESCRIPTION
	Creates a new function project.

	  $ func create -l node -t http

	Creates a function in the current directory '.' which is written in the
	language/runtime 'node' and handles HTTP events.

	If [path] is provided, the function is initialized at that path, creating
	the path if necessary.

	To complete this command interactively, use --confirm (-c):
	  $ func create -c

	Available Language Runtimes and Templates:
	  Language     Template
	  --------     --------
	  go           cloudevents
	  go           http
	  go           tanstic/improve
	  go           tanstic/redis
	  go           tanstic/uppercase
	  node         cloudevents
	  node         http
	  python       cloudevents
	  python       http
	  quarkus      cloudevents
	  quarkus      http
	  rust         cloudevents
	  rust         http
	  springboot   cloudevents
	  springboot   http
	  springboot   tanstic/uppercase
	  typescript   cloudevents
	  typescript   http


	To install more language runtimes and their templates see 'func repository'.


EXAMPLES
	o Create a Node.js function (the default language runtime) in the current
	  directory (the default path) which handles http events (the default
	  template).
	  $ func create

	o Create a Node.js function in the directory 'myfunc'.
	  $ func create myfunc

	o Create a Go function which handles CloudEvents in ./myfunc.
	  $ func create -l go -t cloudevents myfunc


```
func create
```

### Options

```
  -c, --confirm             Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)
  -h, --help                help for create
  -l, --language string     Language Runtime (see help text for list) (Env: $FUNC_LANGUAGE)
  -r, --repository string   URI to a Git repository containing the specified template (Env: $FUNC_REPOSITORY)
  -t, --template string     Function template. (see help text for list) (Env: $FUNC_TEMPLATE) (default "http")
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

