## func completion

Output functions shell completion code

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
      --json   Output results as JSON ($FUNC_JSON)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

