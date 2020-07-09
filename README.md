# faas

[![Main Build Status](https://github.com/boson-project/faas/workflows/Main/badge.svg?branch=main)](https://github.com/boson-project/faas/actions?query=workflow%3AMain+branch%3Amain)
[![Develop Build Status](https://github.com/boson-project/faas/workflows/Develop/badge.svg?branch=develop&label=develop)](https://github.com/boson-project/faas/actions?query=workflow%3ADevelop+branch%3Adevelop)
[![Documentation](https://godoc.org/github.com/boson-project/faas?status.svg)](http://godoc.org/github.com/boson-project/faas)
[![GitHub Issues](https://img.shields.io/github/issues/boson-project/faas.svg)](https://github.com/boson-project/faas/issues)
[![License](https://img.shields.io/github/license/boson-project/faas.svg?maxAge=2592000)](https://github.com/boson-project/faas/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/boson-project/faas.svg?label=Release)](https://github.com/boson-project/faas/releases)


Function as a Service CLI

## Setup and Configuration

With Go 1.13+ installed, build and install the binary to your path:
```
go install ./cmd/faas
```

Install Docker

* `docker` https://docs.docker.com/get-docker/

Configure Image repository:

Both the image repository and user/org namespace need to be defined either by
using the --registry and --namespace flags on the `create` command, or by
configuring as environment variables.  For example to configure all images
to be pushed to `quay.io/alice`, use:
```
export FAAS_REGISTRY=quay.io
export FAAS_NAMESPACE=alice
```

Cluster connection:

It is expected that kubectl and kn be configured to connect to a kubernetes cluster with the following configuration:

* Knative Serving and Eventing
* Knative Domains patched to enable your chosen domain
* Knative Network patched to enable subdomains
* Kourier
* Cert-manager

see https://github.com/boson-project/config for cluster setup and configuration details.

## Usage

See help:
```shell
faas
```
## Examples

Create a new Function:

```shell
> mkdir -p example.com/www
> cd example.com/www
> faas create go
https://www.example.com
> curl https://www.example.com
OK
```


