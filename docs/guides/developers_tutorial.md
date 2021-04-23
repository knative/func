# Boson Functions: A Step By Step Tutorial

This document will walk you step by step through the process of creating,
editing, and deploying a Boson Function project.

## Prerequisites

In order to follow along with this tutorial, you will need to have a few tools
installed.

* [oc][oc] or [kubectl][kubectl] CLI
* [kn][kn] CLI
* [Docker][docker] 

[docker]: https://docs.docker.com/install/
[oc]: https://docs.openshift.com/container-platform/4.6/cli_reference/openshift_cli/getting-started-cli.html#cli-installing-cli_cli-developer-commands
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[kn]: https://knative.dev/docs/install/install-kn/

## Cluster Setup

To use Boson Functions, you'll need a Kubernetes cluster with Knative Serving
and Eventing installed. If you have a recent version of OpenShift, you can
simply install the Serverless Operator. If you don't have a cluster already,
you can create a simple cluster with [kind](https://kind.sigs.k8s.io/). Follow
these [step by step instructions](kind-setup.md) to install on your local
machine.

## Boson Tooling

The primary interface for Boson project is the `func` CLI.
[Download][func-download] the most recent version and install it some place
within your `$PATH`.

[func-download]: https://github.com/boson-project/func/releases/latest

```sh
# Be sure to download the correct binary for your operating system
curl -L -o - func.gz https://github.com/boson-project/func/releases/latest/download/func_linux_amd64.gz | gunzip > func && chmod 755 func
sudo mv func /usr/local/bin
```
## Configuring a Container Registry

The unit of deployment in Boson Functions is an [OCI](https://opencontainers.org/)
container image, typically referred to as a Docker container image.

In order for the `func` CLI to manage these containers, you'll need to be
logged in to a container registry. For example, `docker.io/lanceball`


```bash
# Typically, this will log you in to docker hub if you
# omit <registry.url>. If you are using a registry
# other than Docker hub, provide that for <registry.url>
docker login -u lanceball -p [redacted] <registry.url>
```

> Note: many of the `func` CLI commands take a `--registry` argument.
> Set the `FUNC_REGISTRY` environment variable in order to omit this
> parameter when using the CLI.

```bash
# This should be set to a registry that you have write permission
# on and you have logged into in the previous step.
export FUNC_REGISTRY=docker.io/lanceball
```

## Creating a Project

With your Knative enabled cluster up and running, you can now create a new
Function Project. Let's start by creating a project directory. Function names
in `func` correspond to URLs at the moment, and there are some finicky cases
at the moment. To ensure that everything works as it should, create a project
directory consisting of three URL parts. Here is a good one.

```bash
mkdir fn.example.io
cd fn.example.io
```

Now, we will create the project files, build a container, and
deploy the function as a Knative service.


```bash
func create -l node
func build
func deploy
```

This will create a Node.js Function project in the current directory accepting
all of the defaults inferred from your environment, for example`$FUNC_REGISTRY`.
When the command has completed, you can see the deployed function.

```bash
kn service list
NAME            URL                                          LATEST                  AGE   CONDITIONS   READY   REASON
fn-example-io   http://fn-example-io.func.127.0.0.1.nip.io   fn-example-io-ngswh-1   24s   3 OK / 3     True
```

Clicking on the URL will take you to the running function in your cluster. You
should see a simple response.

```json
{"query": {}}
```

You can add query parameters to the request to see those echoed in return.

```console
curl "http://fn-example-io.func.127.0.0.1.nip.io?name=tiger"
{"query":{"name":"tiger"},"name":"tiger"}
```

## Local Development

The `func build` command results in a docker container that can be run
locally with container ports mapped to localhost.

```bash
func run
```

For day to day development of the function, you can also run it locally outside
of a container. For this project, using Node.js, you have the following commands
available. Note that to run this function locally, you will need Node.js 12.x or
higher, and the corresponding npm.

```bash
npm install # Installs all dependencies
npm test # Runs unit and integration test suites
npm run local # Execute the function on the local host
```

## Deploying to a Cluster - Step by Step

With `func deploy`, you have already deployed to a cluster! But there was a lot
of magic. Let's break it down step by step using the
`func` CLI to take each step in turn.

First, let's delete the project we just created.

```bash
func delete
```

You might see a message such as this.

```bash
Error: remover failed to delete the service: timeout: service 'fn-example-io' not ready after 30 seconds.
```

If you do, just run `kn service list` to see if the function is still deployed.
It might just take a little time for it to be removed.

Now, let's clean up the current directory.

```bash
rm -rf *
```

### `func create`

To create a new project structure without building a container or deploying to a
cluster, use the `create` command.

```bash
func create -l node -t http
```

You can also create a Quarkus or a Golang project by providing `quarkus` or `go`
respectively to the `-l` flag. To create a project with a template for
CloudEvents, provide `events` to the `-t` flag.

### `func build`

To build the OCI container image for your function project, you can use the
`build` command.

```bash
func build
```

This creates a runnable container image that listens on port 8080 for incoming
HTTP requests.

### `func deploy`

To deploy the image to your cluster, use the `deploy` command. You can also use
this command to update a Function deployment after making changes locally.

```bash
func deploy
```
