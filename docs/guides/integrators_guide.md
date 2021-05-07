# Integrator's Guide

Developers can integrate directly with the Function system using the client library upon which the `func` CLI is based.  Before beginning this section, you should have a configured provider and be familiar with the topics covered in the [developer's guide](developers_guide.md).

## Using the Client Library

To create a Client which uses the included buildpacks-based function builder, pushes to a Quay.io registry function container artifacts and deploys to a Knative enabled cluster: 
```go
package main

import (
	fn "github.com/boson-project/func"
	"github.com/boson-project/func/buildpacks"
	"github.com/boson-project/func/docker"
	"github.com/boson-project/func/knative"
	"log"
)

func main() {
	pusher, err := docker.NewPusher()
	if err != nil {
		log.Fatal(err)
	}
	deployer, err := knative.NewDeployer("")
	if err != nil {
		log.Fatal(err)
	}
	// A client which uses embedded function templates,
	// Quay.io/alice for interstitial build artifacts.
	// Docker to build and push, and a Knative client for deployment.
	client := fn.New(
		fn.WithBuilder(buildpacks.NewBuilder()),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer),
		fn.WithRegistry("quay.io/alice"))

	// Create a Go function which listens for CloudEvents.
	// Publicly routable as https://www.example.com.
	// Local implementation is written to the current working directory.
	funcTest := fn.Function{
		Runtime: "go",
		Trigger: "events",
		Name: "my-function",
		Image: "quay.io/alice/my-function",
		Root: "my-function",
	}
	if err := client.Create(funcTest); err != nil {
		log.Fatal(err)
	}
}
```



