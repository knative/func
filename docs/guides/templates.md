# Templates

When a Function is created, an example implementation and a Function metadata file are written into the new Function's working directory.  Together, these files are referred to as the Function's Template.  Included are the templates 'http' and 'events' for each supported language runtime.

These embedded templates are minimal by design.  The Function contains a minimum of external dependencies, and the 'func.yaml' defines a final environment within which the Funciton will execute that is devoid of any extraneous packages or services.

To make use of more complex initial Function implementions, or to define runtime environments with arbitrarily complex requirements, the templates system is fully pluggable.

## External Git Repositories

When creating a new Function, a Git repository can be specified as the source for the template files.  For example, the Boson Project maintains a set of example Functions at https://github.com/boson-project/templates which can be used during project creation.

For example, the Boson Project Templates repository contains an example "Hello World" Function implementation in each of the officially supported languages.  To use this template via the CLI, use the flags:

func create <name> --template hello-world --repository https://github.com/boson-project/templates

## Locally Installing Repositories

Template Repositories can also be installed locally by placing them in the Functions configuration directory.  

To install the Boson Project templates locally, for example, clone the repository and name it `boson` using `git clone https://github.com/boson-project/templats ~/.config/func/repositories/boson`

Once installed, the Boson Hello World template can be specified:

func create <name> --template boson/hello-world

## Language Packs

In addition to example implementations, a template includes a `func.yaml` which includes metadata about the Function.  By default this is populated with things like the new Function's name.  It also includes a reference to the specific tooling which compiles and packages the Function into its deployable form.  This is called the Builder.  By customizing this metadata, it is more than just a template; it is referred to as a Language Pack. See [Project Configuration with func.yaml](func_yaml.md).

A Language Pack can support additional function signatures and can fully customize the environment of the final running Function.  For more information see the [Language Pack Guide](language-packs.md).









