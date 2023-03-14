## func config

Manage global system settings

### Synopsis


NAME
	func - Manage global configuration

SYNOPSIS
	func config list [-o|--output] [-c|--confirm] [-v|--verbose]
	func config get <name> [-o|--output] [-c|--confirm] [-v|--verbose]
	func config set <name> <value> [-o|--output] [-c|--confirm] [-v|--verbose]
	func config unset <name> [-o|--output] [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Manage global configuration by listing current configuration, and either
	retreiving individual settings with 'get' or setting with 'set'.
	Values set will become the new default for all function commands.

	Settings are stored in ~/.config/func/config.yaml by default, but whose path
	can be altered using the XDG_CONFIG_HOME environment variable (by default
	~/.config).

	A specific global config file can also be used by specifying the environment
	variable FUNC_CONFIG_FILE, which takes highest precidence.

	Values defined in this global configuration are only defaults.  If a given
	function has a defined value for the option, that will be used. This value
	can also be superceded with environment variables or command line flags.  To
	see the final value of an option that will be used for a given command, see
	the given command's help text.

COMMANDS

	With no arguments, this help text is shown.  To manage global configuration
	using an interactive prompt, use the --confirm (-c) flag.
	  $ func config -c

	list
	  List all global configuration options and their current values.

	get
	  Get the current value of a named global configuration option.

	set
	  Set the named global configuration option to the value provided.

	unset
	  Remove any user-provided setting for the given option, resetting it to the
	  original default.

EXAMPLES
	o List all configuration options and their current values
	  $ func config list

	o Set a new global default value for Reggistry
	  $ func config set registry registry.example.com/bob

	o Get the current default value for Registry
	  $ func config get registry
		registry.example.com/bob

	o Unset the custom global registry default, reverting it to the static
	  default (aka factory default).  In the case of registry, there is no
	  static default, so unsetting the default will result in the 'deploy'
	  command requiring it to be provided when invoked.
	  $ func config unset registry



```
func config [list|get|set|unset]
```

### Options

```
  -c, --confirm         Prompt to confirm options interactively (Env: $FUNC_CONFIRM)
  -h, --help            help for config
  -o, --output string   Output format (human|json) (Env: $FUNC_OUTPUT) (default "human")
  -v, --verbose         Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func config get](func_config_get.md)	 - Get a global configuration option
* [func config list](func_config_list.md)	 - List global configuration options
* [func config set](func_config_set.md)	 - Get a global configuration option
* [func config unset](func_config_unset.md)	 - Get a global configuration option

