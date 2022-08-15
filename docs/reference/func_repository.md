## func repository

Manage installed template repositories

### Synopsis


NAME
	{{.Name}} - Manage set of installed repositories.

SYNOPSIS
	{{.Name}} repo [-c|--confirm] [-v|--verbose]
	{{.Name}} repo list [-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo add <name> <url>[-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo rename <old> <new> [-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo remove <name> [-r|--repositories] [-c|--confirm] [-v|--verbose]

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
		$ {{.Name}} create -l go -t http

	The Repository Flag:
	Installing repositories locally is optional.  To use a template from a remote
	repository directly, it is possible to use the --repository flag on create.
	This leaves the local disk untouched.  For example, To create a function using
	the Boson Project Hello-World template without installing the template
	repository locally, use the --repository (-r) flag on create:
		$ {{.Name}} create -l go \
			--template hello-world \
			--repository https://github.com/boson-project/templates

	Alternative Repositories Location:
	Repositories are stored on disk in ~/.config/func/repositories by default.
	This location can be altered by setting the FUNC_REPOSITORIES_PATH
	environment variable.


COMMANDS

	With no arguments, this help text is shown.  To manage repositories with
	an interactive prompt, use the use the --confirm (-c) flag.
	  $ {{.Name}} repository -c

	add
	  Add a new repository to the installed set.
	    $ {{.Name}} repository add <name> <URL>

	  For Example, to add the Boson Project repository:
	    $ {{.Name}} repository add boson https://github.com/boson-project/templates

	  Once added, a function can be created with templates from the new repository
	  by prefixing the template name with the repository.  For example, to create
	  a new function using the Go Hello World template:
	    $ {{.Name}} create -l go -t boson/hello-world

	list
	  List all available repositories, including the installed default
	  repository.  Repositories available are listed by name.  To see the URL
	  which was used to install remotes, use --verbose (-v).

	rename
	  Rename a previously installed repository from <old> to <new>. Only installed
	  repositories can be renamed.
	    $ {{.Name}} repository rename <name> <new name>

	remove
	  Remove a repository by name.  Removes the repository from local storage
	  entirely.  When in confirm mode (--confirm) it will confirm before
	  deletion, but in regular mode this is done immediately, so please use
	  caution, especially when using an altered repositories location
	  (via the FUNC_REPOSITORIES_PATH environment variable).
	    $ {{.Name}} repository remove <name>

EXAMPLES
	o Run in confirmation mode (interactive prompts) using the --confirm flag
	  $ {{.Name}} repository -c

	o Add a repository and create a new function using a template from it:
	  $ {{.Name}} repository add boson https://github.com/boson-project/templates
	  $ {{.Name}} repository list
	  default
	  boson
	  $ {{.Name}} create -l go -t boson/hello-world
	  ...

	o List all repositories including the URL from which remotes were installed
	  $ {{.Name}} repository list -v
	  default
	  boson	https://github.com/boson-project/templates

	o Rename an installed repository
	  $ {{.Name}} repository list
	  default
	  boson
	  $ {{.Name}} repository rename boson boson-examples
	  $ {{.Name}} repository list
	  default
	  boson-examples

	o Remove an installed repository
	  $ {{.Name}} repository list
	  default
	  boson-examples
	  $ {{.Name}} repository remove boson-examples
	  $ {{.Name}} repository list
	  default


```
func repository [flags]
```

### Options

```
  -c, --confirm   Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)
  -h, --help      help for repository
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions
* [func repository add](func_repository_add.md)	 - Add a repository
* [func repository list](func_repository_list.md)	 - List repositories
* [func repository remove](func_repository_remove.md)	 - Remove a repository
* [func repository rename](func_repository_rename.md)	 - Rename a repository

###### Auto generated by spf13/cobra on 15-Aug-2022
