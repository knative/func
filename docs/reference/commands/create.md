
## func create

Create a new function project.

### Synopsis

$ `func create [-l|--language] [-t|--template] [-r|--repository] [-c|--confirm]  [-v|--verbose]  [path]`

### Description

	Creates a new function project.

`$ func create -l node -t http`

	Creates a function in the current directory '.' which is written in the
	language/runtime 'node' and handles HTTP events.

	If [path] is provided, the function is initialized at that path, creating
	the path if necessary.

	To complete this command interactivly, use --confirm (-c):
`$ func create -c`

### Templates

	Available language runtimes and templates:

	  Language     Template
	  --------     --------
	  go           cloudevents
	  go           http
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
	  typescript   cloudevents
	  typescript   http
	  

	To install more language runtimes and their templates see `func repository`.

### Examples

- Create a Node.js function (the default language runtime) in the current directory (the default path) which handles http events (the default template).

`$ func create`

- Create a Node.js function in the directory 'myfunc'.

`$ func create myfunc`

- Create a Go function which handles CloudEvents in ./myfunc.

`$ func create -l go -t cloudevents myfunc`
