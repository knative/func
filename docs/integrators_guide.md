# Integrator's Guide

Developer's can integrate directly with the Function system using the client library upon which the `function` CLI is based.  Before beginning this section, you should have a configured provider and be familiar with the topics covered in the [developer's guide](docs/developers_guide.md).

## Using the Client Library

To create a Client which uses the included buildpacks-based function builder, pushes to a Quay.io registry function container artifacts and deploys to a Knative enabled cluster: 
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
    faas.WithBuilder(buildpacks.NewBuilder("quay.io/alice/my-function")),
    faas.WithPusher(docker.NewPusher()),
    faas.WithDeployer(knative.NewDeployer()))

  // Create a Go function which listens for CloudEvents.
  // Publicly routable as https://www.example.com.
  // Local implementation is written to the current working directory.
  if err := client.Create("go", "events", "my-function", "quay.io/alice/my-function:v1.0"); err != nil {
    log.Fatal(err)
  }
}
```



