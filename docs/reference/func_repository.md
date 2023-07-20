## func repository

Manage installed template repositories

### Synopsis


NAME
	func - Manage set of installed repositories.

SYNOPSIS
	func repo [-c|--confirm] [-v|--verbose]
	func repo list [-r|--repositories] [-c|--confirm] [-v|--verbose]
	func repo add <name> <url>[-r|--repositories] [-c|--confirm] [-v|--verbose]
	func repo rename <old> <new> [-r|--repositories] [-c|--confirm] [-v|--verbose]
	func repo remove <name> [-r|--repositories] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Manage template repositories installed on disk at either the default location
	(~/.config/func/repositories) or the location specified by the --repository
	flag.  Once added, a template from the repository can be used when creating
	a new function.

	Interactive Prompts:
	To complete these commands interactively, pass the --confirm (-c) flag to
	the 'repository' command, or any of the inidivual subcommands.

	The Default Repository:
	The default repository is not stored on disk, but embedded in the binary and
	can be used without explicitly specifying the name.  The default repository
	is always listed first, and is assumed when creating a new function without
	specifying a repository name prefix.
	For example, to create a new Go function using the 'http' template from the
	default repository.
		$ func create -l go -t http

	The Repository Flag:
	Installing repositories locally is optional.  To use a template from a remote
	repository directly, it is possible to use the --repository flag on create.
	This leaves the local disk untouched.  For example, To create a function using
	the Boson Project Hello-World template without installing the template
	repository locally, use the --repository (-r) flag on create:
		$ func create -l go \
			--template hello-world \
			--repository https://github.com/boson-project/templates

	Alternative Repositories Location:
	Repositories are stored on disk in ~/.config/func/repositories by default.
	This location can be altered by setting the FUNC_REPOSITORIES_PATH
	environment variable.


COMMANDS

	With no arguments, this help text is shown.  To manage repositories with
	an interactive prompt, use the use the --confirm (-c) flag.
	  $ func repository -c

	add
	  Add a new repository to the installed set.
	    $ func repository add <name> <URL>

	  For Example, to add the Boson Project repository:
	    $ func repository add boson https://github.com/boson-project/templates

	  Once added, a function can be created with templates from the new repository
	  by prefixing the template name with the repository.  For example, to create
	  a new function using the Go Hello World template:
	    $ func create -l go -t boson/hello-world

	list
	  List all available repositories, including the installed default
	  repository.  Repositories available are listed by name.  To see the URL
	  which was used to install remotes, use --verbose (-v).

	rename
	  Rename a previously installed repository from <old> to <new>. Only installed
	  repositories can be renamed.
	    $ func repository rename <name> <new name>

	remove
	  Remove a repository by name.  Removes the repository from local storage
	  entirely.  When in confirm mode (--confirm) it will confirm before
	  deletion, but in regular mode this is done immediately, so please use
	  caution, especially when using an altered repositories location
	  (via the FUNC_REPOSITORIES_PATH environment variable).
	    $ func repository remove <name>

EXAMPLES
	o Run in confirmation mode (interactive prompts) using the --confirm flag
	  $ func repository -c

	o Add a repository and create a new function using a template from it:
	  $ func repository add functastic https://github.com/knative-sandbox/func-tastic
	  $ func repository list
	  default
	  functastic
	  $ func create -l go -t functastic/hello-world
	  ...

		o Add a repository specifying the branch to use (metacontroller):
	  $ func repository add metacontroller https://github.com/knative-sandbox/func-tastic#metacontroler
	  $ func repository list
	  default
	  metacontroller
	  $ func create -l node -t metacontroller/metacontroller
	  ...

	o List all repositories including the URL from which remotes were installed
	  $ func repository list -v
	  default
	  metacontroller	https://github.com/knative-sandbox/func-tastic#metacontroller

	o Rename an installed repository
	  $ func repository list
	  default
	  boson
	  $ func repository rename boson functastic
	  $ func repository list
	  default
	  functastic

	o Remove an installed repository
	  $ func repository list
	  default
	  functastic
	  $ func repository remove functastic
	  $ func repository list
	  default


```
func repository
```

### Options

```
  -c, --confirm   Prompt to confirm options interactively ($FUNC_CONFIRM)
  -h, --help      help for repository
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func repository add](func_repository_add.md)	 - Add a repository
* [func repository list](func_repository_list.md)	 - List repositories
* [func repository remove](func_repository_remove.md)	 - Remove a repository
* [func repository rename](func_repository_rename.md)	 - Rename a repository

