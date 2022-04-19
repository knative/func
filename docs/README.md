# Boson Functions

### Telegram Image Analysis Demo Screencast
[![Telegram Image Analysis Demo Screencast](http://img.youtube.com/vi/CsYo0SmQ0Uk/0.jpg)](https://youtu.be/CsYo0SmQ0Uk "Telegram Image Analysis Demo Screencast")

`func` is a Client Library and CLI for enabling the development of implicitly deployed, platform agnostic functions.

Functions can be written in the following languages:

* Go (Golang)
* Node.js (JavaScript)
* Quarkus (Java)
* SpringBoot (Java)
* Python
* Rust

Functions can be deployed on the following platforms:

* Kubernetes
* OpenShift
* Localhost

<!--
[Quickstart Video]
-->
## Client Installation

[Install the latest CLI](installing_cli.md)

Functions can be created and managed using the CLI interactively, scripted, or by direct integration with the client library. The [Function Developer's Guide](function-developers/developers_guide.md) and examples herein demonstrate the CLI-based approach.  

For direct integration using the Go client library, it is advisible to first follow these CLI-based guides to become familiar with creating and deploying software in this way, and then proceed to the [Function Integrator's Guide](reference/integrators_guide.md).

## Platform Configuration

[Getting Started with Kubernetes](getting_started_kubernetes.md)

[Getting Started on Localhost](getting_started_localhost.md)

Functions are portable between different infrastructure configurations.  While your Function itself remains the same, the platform upon which it is deployed will provide different services and guarantees.  For instance, a Function deployed to your local host will not autoscale, nor be highly available nor externally routable by default.  Deploying to a properly configured Kubernetes cluster would however provide these features.  There is also variance within infrastrucutre types.  For instance, a small kubernetes cluster will be limited in the amount of resources which will be ultimately available for allocation to your Function.

## Function Development

[Function Developer's Guide](function-developers/developers_guide.md)

Any code which provides one of a set of supported function signatures can be deployed to any of the supported platforms using this client library.  No process boundary code, container, or configuration outside of the function itself is required.

At their most fundamental, a Function is a set of instructions which export a public function whose method signature conforms to one of the supported forms.  It is implicitly deployed to a supported platform when created using the client library, and can be migrated between platforms without code changes.  Runtime execution is handled by the platform, which may offer guarantees such as autoscaling and load balancing.  

## Contributing

We are always looking for contributions to the project from the Function Developer community.  For more information on how to participate, see the [Contributing Guide](CONTRIBUTING.md)

