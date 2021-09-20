# Language Packs

A Language Pack is the mechanism by which the Functions binary can be extended to support additional runtimes, function signatures, even operating systems and installed tooling for a function. A Language Pack is a directory, typically named for the language or runtime being templated, and includes

- one or more directories containing templates for the Language Pack's recognized function signatures
- each of which contains a `manifest.yaml` file with metadata about the template
- tests and documentation

For example, a Language Pack directory for Ruby with templates for both
a CloudEvent function signature and an HTTP function signature, may look
similar to the following directory tree.

```
ruby
├── cloudevent
│   ├── func.rb
│   ├── Gemfile
│   ├── manifest.yaml
│   ├── Rakefile
│   ├── README.md
│   └── test.rb
└── http
    ├── func.rb
    ├── Gemfile
    ├── manifest.yaml
    ├── Rakefile
    ├── README.md
    └── test.rb
```

## `manifest.yaml`

The `manifest.yaml` file contains metadata that Language Pack providers
may include to configure the build and deployment of function projects
created with the Language Pack. The following fields are recognized.

### `builders`
REQUIRED: A set of key value pairs identifying builder images capable of
building a project from this Language Pack. The `default` key will be
set as the builder image in `func.yaml` for a newly created project from
the template.

```
builders:
  default: paketobuildpacks/builder:base
  base: paketobuildpacks/builder:base
  full: paketobuildpacks/builder:full
```

### `buildpacks`
OPTIONAL: An ordered list of additional buildpacks to be applied to the
builder image in addition to those already known by the builder.
For example, the Paketo builder is widely used for Node.js, but it does
not include a Buildpack for TypeScript. A Language Pack may still use
the Paketo builder for TypeScript templates, by specifying an additional
buildpack to apply to the Paketo builder when the function is built.

```
builders:
  default: gcr.io/paketo-buildpacks/builder:base
  base: gcr.io/paketo-buildpacks/builder:base
  full: gcr.io/paketo-buildpacks/builder:full

buildpacks:
  - paketo-buildpacks/nodejs
  - ghcr.io/boson-project/typescript-function-buildpack:tip
```

### `healthEndpoints`
OPTIONAL: A set of key value pairs for `liveness` and `readiness`
endpoints for functions created using the language pack. For example

```
healthEndpoints:
  liveness: /health/liveness
  readiness: /health/readiness
```

Built in to the Functions library are basic language packs for Go,
Node.js, Python, Quarkus, Rust, SpringBoot and TypeScript, each of
which provide templates for HTTP and CloudEvents.

## Installing Language Packs

Language Packs are distributed as a set of templates for one or more
languages via template repositories, and installed by the developer
locally using the `func` CLI.

```
func repository add boson https://github.com/boson-project/templates
func create -l go -t boson/hello-world
```

See the `repository` section of the [commands guide](commands.md)
for more information on installing and managing Language Pack 
repositories.
