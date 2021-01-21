# Integrator's Guide

Developer's can integrate directly with the Function system using the client library upon which the `func` CLI is based.  Before beginning this section, you should have a configured provider and be familiar with the topics covered in the [developer's guide](docs/developers_guide.md).

## Using the Client Library

To create a Client which uses the included buildpacks-based function builder, pushes to a Quay.io registry function container artifacts and deploys to a Knative enabled cluster: 
```go
package main

import (
  "log"

  bosonFunc "github.com/boson-project/func"
  "github.com/boson-project/func/buildpacks"
  "github.com/boson-project/func/docker"
  "github.com/boson-project/func/embedded"
  "github.com/boson-project/func/knative"
)

func main() {
  // A client which uses embedded function templates,
  // Quay.io/alice for interstitial build artifacts.
  // Docker to build and push, and a Knative client for deployment.
  client, err := bosonFunc.New(
    bosonFunc.WithInitializer(embedded.NewInitializer("")),
    bosonFunc.WithBuilder(buildpacks.NewBuilder("quay.io/alice/my-function")),
    bosonFunc.WithPusher(docker.NewPusher()),
    bosonFunc.WithDeployer(knative.NewDeployer()))

  // Create a Go function which listens for CloudEvents.
  // Publicly routable as https://www.example.com.
  // Local implementation is written to the current working directory.
  if err := client.Create("go", "events", "my-function", "quay.io/alice/my-function:v1.0"); err != nil {
    log.Fatal(err)
  }
}
```



