# Templates

When a function is created, an example implementation and a function metadata file are written into the new function's working directory.  Together, these files are referred to as the function's template.  Included are the templates 'http' and 'events' for each supported language runtime.

These embedded templates are minimal by design.  The function contains a minimum of external dependencies, and the `func.yaml` file defines a final environment within which the function will execute that is devoid of any extraneous packages or services.

To make use of more complex initial function implementions, or to define runtime environments with arbitrarily complex requirements, the templates system is fully pluggable.

## External Git Repositories

When creating a new function, a Git repository can be specified as the source for the template files.  For example, the the [knative-sandbox/func-tastic repository](https://github.com/knative-sandbox/func-tastic) contains a set of example functions which can be used during project creation.

For example, the func-tastic repository contains an example ["metacontroller"](https://metacontroller.github.io/metacontroller) function implementation for Node.js.  To use this template via the CLI, use the flags:

func create <name> --template metacontroller --repository https://github.com/knative-sandbox/func-tastic

## Locally Installing Repositories

Template repositories can also be installed locally by placing them in the functions configuration directory.

To install the func-tastic templates locally, for example, use the `func repository add` command:

```
func repository add https://github.com/knative-sandbox/func-tastic
```

Once installed, the metacontroller template can be specified:

func create <name> --template func-tastic/metacontroller

## Language Packs

In addition to example implementations, a template includes a `func.yaml` which includes metadata about the function.  By default this is populated with things like the new function's name.  It also includes a reference to the specific tooling which compiles and packages the function into its deployable form.  This is called the "builder".  By customizing this metadata, it is more than just a template; it is referred to as a Language Pack.

A Language Pack can support additional function signatures and can fully customize the environment of the final running Function.  For more information see the [Language Pack Guide](language-pack-contract.md).









