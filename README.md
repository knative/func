# faas

[![Main Build Status](https://github.com/boson-project/faas/workflows/Main/badge.svg?branch=main)](https://github.com/boson-project/faas/actions?query=workflow%3AMain+branch%3Amain)
[![Develop Build Status](https://github.com/boson-project/faas/workflows/Develop/badge.svg?branch=develop&label=develop)](https://github.com/boson-project/faas/actions?query=workflow%3ADevelop+branch%3Adevelop)
[![Documentation](https://godoc.org/github.com/boson-project/faas?status.svg)](http://godoc.org/github.com/boson-project/faas)
[![GitHub Issues](https://img.shields.io/github/issues/boson-project/faas.svg)](https://github.com/boson-project/faas/issues)
[![License](https://img.shields.io/github/license/boson-project/faas)](https://github.com/boson-project/faas/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/boson-project/faas.svg?label=Release)](https://github.com/boson-project/faas/releases)


Function as a Service CLI and Client Library for KNative-enabled Kubernetes Clusters.

## Local Setup and Configuration

Docker is required unless the --local flag is explicitly provided on creation
of a new function.

It is recommended to set your preferred image registry for publishing Functions
by setting the following environment variables:
```
export FAAS_REGISTRY=quay.io
export FAAS_NAMESPACE=alice
```
Alternately, these values can be provided using the --namespace and --registry 
flags when running the CLI.

## Cluster Setup and Configuration

It is assumed that the local system has a kubectl configuration set up to 
connect to a Kubernetes cluster with the following configuration:

* Knative Serving and Eventing Installed
* Knative Domains patched to enable your chosen domain
* Knative Network patched to enable subdomains
* Kourier 
* (optionally) Cert-manager for HTTPS routes

See https://github.com/boson-project/config for cluster setup and configuration notes.

## Running the CLI

The CLI can be run either by building and installing manually, by running
one of the published containers, or using the appropriate pre-built binary
releases.

## Build and Install

With Go 1.13+ installed, build and install the binary to your path:
```
go install ./cmd/faas
```
### Docker 

Each tag has an assoicated container which can be run via:
```
docker run quay.io/boson/faas:v0.2.2
```

### Pre-built Binary Releases

Coming soon.

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
## Using the Client Library

To create a Client which uses the included buildpacks-based function builder, pushes to a Quay.io repository function container artifacts and deploys to a Knative enabled cluster: 
```go
package main

import (
  "log"

  "github.com/boson-project/faas"
  "github.com/boson-project/faas/buildpacks"
  "github.com/boson-project/faas/docker"
  "github.com/boson-project/faas/embedded"
  "github.com/boson-project/faas/knative"
)

func main() {
  // A client which uses embedded function templates,
  // Quay.io/alice for interstitial build artifacts.
  // Docker to build and push, and a Knative client for deployment.
  client, err := faas.New(
    faas.WithInitializer(embedded.NewInitializer("")),
    faas.WithBuilder(buildpacks.NewBuilder("quay.io", "alice")),
    faas.WithPusher(docker.NewPusher()),
    faas.WithDeployer(knative.NewDeployer()))

  // Create a Go function which listens for CloudEvents.
  // Publicly routable as https://www.example.com.
  // Local implementation is written to the current working directory.
  if err := client.Create("go", "events", "www.example.com", "."); err != nil {
    log.Fatal(err)
  }
}
```

