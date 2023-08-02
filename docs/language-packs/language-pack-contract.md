# Language Packs and Function Templates

Language Packs is the mechanism by which the Knative Functions binary can be extended to support additional runtimes, function signatures, even operating systems and installed tooling for a Function. Language Packs are typically distributed via Git repositories but may also simply exist as a directory on a disc.

A Language Pack is the basis for what is written to the filesystem when a Function developer types `func create`. It can be thought of in a simplistic way as a "template" that provides the Function developer a source code file within which she may write her Function. When a Language Pack template is manifested on the filesystem, it also includes standard supporting files as would exist with traditional projects such as `pom.xml` or `package.json`, as well as a `func.yaml` file which has values derived from metadata supplied in the Language Pack. The Language Pack metadata includes information about the Function language runtime, invocation and build, and is used by the `func` CLI to manage the full Function lifecycle; from creation to deployment.

## Purpose

Knative Function Language Packs are meant to drastically reduce the code required for developers to be productive on Knative, and in concert with the `func` CLI make deploying event driven, container-based Knative Services simple and straightfoward. Language Packs and the `func` CLI streamline a Knative developer's experience by eliminating or reducing developer tasks that are not directly related to solving their business problems.

All of the built-in templates used by `func create` are considered together to be the `default` Language Pack. Vendors, development shops and even individuals may also provide "external" Language Packs of their own in order to augment and extend the `func` CLI.

This document (in)formally describes a Language Pack contract followed by the `default` built-in, and what is expected of externally provided Language Packs.

## Built-in Language Packs

The Language Pack contract is implemented in the following built-in templates.

|Language|Format|
|---|---|
|Go|[CloudEvents](https://github.com/knative/func/tree/main/templates/go/cloudevents)|
|Go|[HTTP](https://github.com/knative/func/tree/main/templates/go/http)|
|Node.js|[CloudEvents](https://github.com/knative/func/tree/main/templates/node/cloudevents)|
|Node.js|[HTTP](https://github.com/knative/func/tree/main/templates/node/http)|
|Python|[CloudEvents](https://github.com/knative/func/tree/main/templates/python/cloudevents)|
|Python|[HTTP](https://github.com/knative/func/tree/main/templates/python/http)|
|Quarkus|[CloudEvents](https://github.com/knative/func/tree/main/templates/quarkus/cloudevents)|
|Quarkus|[HTTP](https://github.com/knative/func/tree/main/templates/quarkus/http)|
|Rust|[CloudEvents](https://github.com/knative/func/tree/main/templates/rust/cloudevents)|
|Rust|[HTTP](https://github.com/knative/func/tree/main/templates/rust/http)|
|Springboot|[CloudEvents](https://github.com/knative/func/tree/main/templates/springboot/cloudevents)|
|Springboot|[HTTP](https://github.com/knative/func/tree/main/templates/springboot/http)|
|TypeScript|[CloudEvents](https://github.com/knative/func/tree/main/templates/typescript/cloudevents)|
|TypeScript|[HTTP](https://github.com/knative/func/tree/main/templates/typescript/http)|

### Built-in Template APIs

The built-in Language Packs support two separate function signatures for each runtime - CloudEvent or HTTP. What exactly this means for each runtime differs due to differences in language syntax and idioms.

## Extensible Language Packs

The Knative Functions project provides the ability to customize its function templates and build strategies through Language Packs. Language Pack authors can add support for additional runtimes, provide templates that expose additional, custom built-in APIs, or any number of other capabilities. The function templates that are built into the `func` binary are considered the "built-in" or default Language Pack. The default, and all external Knative Function Language Packs should conform to the contract described in this document.

## Language Pack Summary

A Knative Function Language Pack provides runtime and invocation capabilities for user-provided Function code.

- A Language Pack must be accessible as a git repository or a path to a location on disk.
- A Language Pack must provide one or more code templates generated via `func create`.
- A Language Pack must expose an invokable function interface for function developers in the code template.
- A Language Pack project must be buildable in the form of an OCI container image via `func build`.
- A Language Pack OCI container image must be runnable via `func run`.
- A Language Pack may provide create, build, runtime and invocation metadata with a `manifest.yaml` file.
- A Language Pack project may be executable natively on a local host via host-specific tooling (e.g. `npm start`).

## Components

A Knative Function Language Pack consists, broadly, of two conceptual components:

- Build and runtime metadata provided via its directory structure and, optionally, a `manifest.yaml` file, all of which support the Function's lifecycle described below.
- Project templates for Functions and supporting code. This is the function developer's UX - a Function project, which in most cases should look just like any other project of its type.

## Build and Runtime Metadata

A Language Pack is a directory of files, typically named for the language or runtime being templated. Its structure is

- The `root` directory containing one or more Language Packs
- A [`manifest.yaml`](#language-pack-manifests) file in the root directory with metadata about the Language Pack
- One or more "runtime" directories in `root` named for the languages or runtimes being templated
- An optional `manifest.yaml` file in the runtime directory which may override metadata provided in a `manifest.yaml` at the `root`
- One or more template directories within a runtime containing templates for the Language Pack's recognized function signatures
- An optional `manifest.yaml` file in each template directory which may override the values set in the `root` or runtime `manifest.yaml` files
- tests and documentation

For example, a Language Pack directory for Ruby with templates for both a CloudEvent function signature and an HTTP function signature, may look similar to the following directory tree.

```
root
├── ruby
|    ├── cloudevent
|    │   ├── func.rb
|    |   ├── manifest.yaml
|    │   ├── Gemfile
|    │   ├── Rakefile
|    │   └── README.md
|    ├── http
|    │   ├── func.rb
|    |   ├── manifest.yaml
|    │   ├── Gemfile
|    │   ├── Rakefile
|    │   └── README.md
|    └── manifest.yaml
└── manifest.yaml
```

### Language Pack Manifests

A Language Pack's root level `manifest.yaml` file contains metadata that Language Pack providers may include to configure the build and deployment of Function projects created with the Language Pack. The following fields are recognized and may be used to override any defaults set in the repository's root directory `manifest.yaml`.

#### `builderImages`

OPTIONAL: A set of key value pairs keying build strategies to builder images capable of building a project from this Language Pack. The `func` CLI is capable of using either the `pack` or `s2i` build strategies. If the Language Pack is using one of the default builtin runtimes: `go`, `nodejs`, `python`, `quarkus`, `springboot` or `rust` then the default Paketo builder (gcr.io/paketo-buildpacks/builder:base) will be used and this field is not necessary. If the Language Pack adds additional runtimes, for example, `csharp` a default builder should be specified using this field.

```
builderImages:
  pack: gcr.io/paketo-buildpacks/builder:base
```

#### `buildpacks`

OPTIONAL: An ordered list of additional buildpacks to be applied to the builder image in addition to those already known by the builder. For example, the Paketo builder is widely used for Node.js, but it does not include a Buildpack for Kotlin. A Language Pack may use the Paketo builder in combination with a custom Kotlin buildpack, by specifying the additional Kotlin buildpack here.

```
builderImages:
  pack: gcr.io/paketo-buildpacks/builder:base

buildpacks:
  - paketo-buildpacks/nodejs
  - ghcr.io/boson-project/kotlin-function-buildpack:tip
```

#### `healthEndpoints`
OPTIONAL: A set of key value pairs for `liveness` and `readiness` endpoints for functions created using the language pack. For example

```
healthEndpoints:
  liveness: /health/liveness
  readiness: /health/readiness
```

If not provided, the values `/health/liveness` and `/health/readiness` will be used by default.

Built in to the Functions library are Language Packs for Go, Node.js, Python, Quarkus, Rust, SpringBoot and TypeScript, each of which provide templates for HTTP and CloudEvents.

### Distributing Language Packs

Language Packs are distributed as a set of templates for one or more languages via Git repositories, and installed by the developer locally using the `func` CLI.

```
func repository add func https://github.com/knative-extensions/func-tastic
func create -l go -t func/hello-world
```

See the `repository` section of the [commands guide](../reference/func.md) for more information on installing and managing Language Pack repositories.

### Repository Manifests

In the root directory of a Language Pack Git repository there may be a `manifest.yaml` file describing the language packs therein. This file can be used to set the default values for builders, buildpacks and health endpoints for all Language Packs within a repository.

```yaml
# The name used for this language pack repository
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

## Lifecycle

- `create` Function projects are created using the `func create` command, during which, template and metadata files are copied from the Language Pack to a directory structure on the developer's local host.
- `build` The Function project is converted into a runnable OCI container image using the `func build` command with metadata provided by the Language Pack's `manifest.yaml` if provided. Any dependencies declared by the Function are installed onto the image filesystem, and the Function invocation code is applied.
- `run` Using the `func run` command to start the image, a controlling process loads the function project into memory and listens on port 8080 for incoming HTTP requests. The process is determined by the Language Pack. For example, a Node.js Language Pack may use `npm start` as the controlling process, while a Go Language Pack may invoke a binary compiled during the `build` phase.
- `invoke` When an incoming HTTP request is received by the controlling process, the CloudEvent, if sent, is unmarshalled and the Function invoked with the payload.
- `response` After a Function has been invoked by the invocation framework, the return value is sent to the caller. If the Function returns a CloudEvent, the invocation framework should respond to the caller with the CloudEvent unchanged. If the Function returns any other data, it is sent to the caller. Function invocation frameworks may each provide their own APIs and specifications to augment a Function developer's experience. For example, the Function developer may be able to return a structure containing a numeric HTTP response code, HTTP headers, and response data. These APIs and specifications are typically unique to the runtime environment and language, and as such are left to Language Pack implementors to provide and document. API capabilities for built-in `default` Language Pack runtimes are documented in the Function templates themselves.

## Execution Scope

When `func create` is used to generate a Function project, the Language Pack provides all of the information necessary for the project to be built as an OCI image. Including for example, specifying use of buildpacks or s2i, what builder images should be used, which environment variables are recognized, and more. In some cases, however, it is possible to simulate the containerized runtime environment locally on a developer's laptop. For example, the built-in `default` Node.js templates can be run locally using standard Node.js development tooling, such as `npm install` and `npm run`. Running Functions in this way can be quite convenient, but it is important to remember that a Knative Function project is meant to run within a tightly controlled execution space, where the environment is well defined. Not all Language Packs can provide this functionality, and there is no guarantee that the Function invocation will be identical in a local environment.

- Install user Function's dependencies
- Load the user Function
- Opening an HTTP socket and listening on a port
- Receive incoming HTTP requests and convert any received CloudEvent into its native form for the runtime

## Usage

Using external Language Packs is made possible through the `func repository` command, which allows Function developers to add and remove Language Packs from their local development environment. For example:

```
❯ func repository add https://github.com/knative-extensions/func-tastic functastic # Add the func-tastic repo to the local environment
❯ func repo list # list repos
default
functastic
❯ func create -l node -t functastic/events # create a Node.js CloudEvents template from the now-local functastic repo
```
