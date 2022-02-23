# Language Packs

A Language Pack is the mechanism by which the Functions binary can be extended
to support additional runtimes, function signatures, even operating systems and
installed tooling for a function. Language Packs are typically distributed via
Git repositories. A Language Pack is a directory within this repository,
typically named for the language or runtime being templated, and includes

- a top level directory named for the language or runtime being templated
- an optional `manifest.yaml` file in the root directory, containing metadata about the Language Pack, which may override metadata provided in a `manifest.yaml` at the repository root
- one or more directories containing templates for the Language Pack's recognized function signatures
- an optional `manifest.yaml` file in each  template's directory, which may override the values potentially set in the language pack root, or even at the repository level.
- tests and documentation

For example, a Language Pack directory for Ruby with templates for both
a CloudEvent function signature and an HTTP function signature, may look
similar to the following directory tree.

```
ruby
├── cloudevent
│   ├── func.rb
│   ├── Gemfile
│   ├── Rakefile
│   └── README.md
├── http
│   ├── func.rb
│   ├── Gemfile
│   ├── Rakefile
│   └── README.md
└── manifest.yaml
```

## Language Pack Manifests

A Language Pack's root level `manifest.yaml` file contains metadata that
Language Pack providers may include to configure the build and deployment
of function projects created with the Language Pack. The following fields
are recognized and may be used to override any defaults set in the repository's
root directory `manifest.yaml`.

### `builders`
OPTIONAL: A set of key value pairs identifying builder images capable of
building a project from this Language Pack. The `default` key will be
set as the builder image in `func.yaml` for a newly created project from
the template. If not set in the Language Pack's `manifest.yaml`, these values
must be provided in repository root `manifest.yaml`

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

If not provided, the values `/health/liveness` and `/health/readiness`
will be used by default.

Built in to the Functions library are basic language packs for Go,
Node.js, Python, Quarkus, Rust, SpringBoot and TypeScript, each of
which provide templates for HTTP and CloudEvents.

## Distributing Language Packs

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

## Repository Manifests

As noted above, Language Packs are distributed via Git repositories.
In the root directory of the repository there may be a `manifest.yaml` file
which describes the language packs therein. This file can be used to set
the default values for builders, buildpacks and health endpoints for all
Language Packs within a repository.

```yaml
# The name used for this language pack repository when referenced
# in the UX, and its version
name: examples

# Optional. Health endpoints for deployed functions in all runtimes.
# May be overridden by manifest.yaml settings at the language level.
healthEndpoints:
  liveness: /health/liveness
  readiness: /health/readiness

# Runtimes is a list of language packs supported by this repository
runtimes:
  - path: go       # Required. The path of the runtime directory from the repository root
    name: go       # Optional. Name of the runtime; if not provided, path will be used

    # A list of templates supplied by this language pack
    templates:     # Required. One or more templates that correspond to directories within this language pack
    - path: events # Required. The path to the template directory from the language pack root
      name: events # Optional. The name of the template; if not provided path will be used
```
