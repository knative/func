# Language Packs

A Language Pack is the mechanism by which the Functions binary can be extended to support additional runtimes, function signatures, even operating systems and installed tooling for a function. A Language Pack includes
- a .builders.yaml file containing a reference to a builder OCI image reference, which conforms to the buildpack builder specification, and contains references to buildpacks supporting this Language Pack
- one or more template directories illustrating the Language Pack's recognized function signatures
- tests and documentation for the templates

Built in to the Functions library is a basic language pack for each supported language.

For an example external language pack, see [https://github.com/lance/gcf-kn]
