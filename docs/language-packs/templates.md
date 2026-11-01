# Templates

When a function is created, an example implementation and a function metadata file are written into the new function's working directory.  Together, these files are referred to as the function's template.  Included are the templates 'http' and 'events' for each supported language runtime.

These embedded templates are minimal by design.  The function contains a minimum of external dependencies, and the `func.yaml` file defines a final environment within which the function will execute that is devoid of any extraneous packages or services.

To make use of more complex initial function implementations, or to define runtime environments with arbitrarily complex requirements, the templates system is fully pluggable.

## External Git Repositories

When creating a new function, a Git repository can be specified as the source for the template files using the `--repository` flag. Use `--template` to select a template from that repository.

## Locally Installing Repositories

Template repositories can also be installed locally by placing them in the functions configuration directory.

Use the [`func repository add`](../reference/func_repository.md) command to install a template repository locally. Once installed, select a template by prefixing its name with the repository name: `--template <repository-name>/<template-name>`.

## Language Packs

In addition to example implementations, a template includes a `func.yaml` which includes metadata about the function.  By default this is populated with things like the new function's name.  It also includes a reference to the specific tooling which compiles and packages the function into its deployable form.  This is called the "builder".  By customizing this metadata, it is more than just a template; it is referred to as a Language Pack.

A Language Pack can support additional function signatures and can fully customize the environment of the final running Function.  For more information see the [Language Pack Guide](language-pack-contract.md).









