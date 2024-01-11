## func subscribe

Subscribe a function to events

### Synopsis

Subscribe a function to events

Subscribe the function to a set of events, matching a set of filters for Cloud Event metadata
and a Knative Broker from where the events are consumed.


```
func subscribe
```

### Examples

```

# Subscribe the function to the 'default' broker where  events have 'type' of 'com.example'
and an 'extension' attribute for the value 'my-extension-value'.
func subscribe --filter type=com.example --filter extension=my-extension-value

# Subscribe the function to the 'my-broker' broker where  events have 'type' of 'com.example'
and an 'extension' attribute for the value 'my-extension-value'.
func subscribe --filter type=com.example --filter extension=my-extension-value --source my-broker

```

### Options

```
  -f, --filter stringArray   Filter for the Cloud Event metadata
  -h, --help                 help for subscribe
  -p, --path string          Path to the function.  Default is current directory ($FUNC_PATH)
  -s, --source string        The source, like a Knative Broker (default "default")
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

