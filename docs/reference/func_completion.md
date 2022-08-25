## func completion

Generate completion scripts for bash, fish and zsh

### Synopsis

To load completion run

For zsh:
source &lt;(func completion zsh)

If you would like to use alias:
alias f=func
compdef _func f

For bash:
source &lt;(func completion bash)



```
func completion <bash|zsh|fish> [flags]
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
  -n, --namespace string   The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)
  -v, --verbose            Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

