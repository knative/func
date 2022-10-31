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
func completion <bash|zsh|fish>
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
  -v, --verbose   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - Serverless functions

