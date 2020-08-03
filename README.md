# faas

[![Main Build Status](https://github.com/boson-project/faas/workflows/Main/badge.svg?branch=main)](https://github.com/boson-project/faas/actions?query=workflow%3AMain+branch%3Amain)
[![Develop Build Status](https://github.com/boson-project/faas/workflows/Develop/badge.svg?branch=develop&label=develop)](https://github.com/boson-project/faas/actions?query=workflow%3ADevelop+branch%3Adevelop)
[![Client API Documentation](https://godoc.org/github.com/boson-project/faas?status.svg)](http://godoc.org/github.com/boson-project/faas)
[![GitHub Issues](https://img.shields.io/github/issues/boson-project/faas.svg)](https://github.com/boson-project/faas/issues)
[![License](https://img.shields.io/github/license/boson-project/faas)](https://github.com/boson-project/faas/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/boson-project/faas.svg?label=Release)](https://github.com/boson-project/faas/releases)

[Demo Screencast]

faas is a "Function as a Service" Client Library and CLI for enabling the development of implicitly deployed, platform agnostic code.

For examples of what's possible, see the [Screencast Series](docs/getting_started_screencast.md) or the [Functions Cookbook](docs/functions_cookbook.md).

Functions can be written in the following languages:

* Go (Golang)
* Node.js (JavaScript)
* Quarkus (Java)
* Rust

Functions can be deployed on the following platforms:

* Kubernetes
* OpenShift
* Localhost

[Quickstart Video]

## Client Installation

[Install the latest CLI](docs/installing_cli.md)

[Install the VS Code Plugin](docs/installing_vscode.md)

[Install the VIM Plugin](docs/installing_vim.md)

[Install the Emacs Extension](docs/installing_emacs.md)

Functions can be created and managed using the CLI interactively, scripted, using one of the IDE plugins, or by direct integration with the client library.   The [Function Developer's Guide](docs/developers_guide.md)and examples herein demonstrate the CLI-based approach.  

For direct integration using the Go client library, it is advisible to first follow these CLI-based guides to become familiar with creating and deploying software in this way, and then proceed to the [Function Integrator's Guide](docs/integrators_guide.md).

## Platform Configuration

[Getting Started with Kubernetes](docs/getting_started_kubernetes.md)

[Getting Started with OpenShift](docs/getting_started_openshift.md)

[Getting Started on Localhost](docs/getting_started_localhost.md)

Functions are portable between different infrastructure configurations.  While your Funciton itself remains the same, the platform upon which it is deployed will provide different services and guarantees.  For instance, a Function deployed to localhost will not autoscale, nor be either highly available or externally routable.  Deploying to a properly configured Kubernetes cluster would however provide these features.  There is also variance within infrastrucutre types.  For instance, a small kubernetes cluster will be limited in the amount of resources which will be ultimately available for allocation to your Function. 

## Function Development

[Function Developer's Guide](docs/developers_guide.md)

Any code which provides one of a set of supported function signatures can be deployed to any of the supported platforms using this client library.  No process boundary code, container, or configuration outside of the function itself is required.

At their most fundamental, a Function is a set of instructions which export a public function whose method signature conforms to one of the supported forms.  It is implicitly deployed to a supported platform when created using the client library, and can be migrated between platforms without code changes.  Runtime execution is handled by the platform, which may offer guarantees such as autoscaling and load balancing.  For more, continue with the Developer's Guide


## Learn More

[Function Architecture](docs/architecture.md)

## Contributing

We are always looking for contributions from the Function Developer community.  For more information on how to participate, see the [Contributor's Guide](docs/contributors_guide.md)

