# faas

Function as a Service CLI

## Requirements

Go 1.13+

## Install

Build and install the resultant binary.
```
go install
```

## Build

Build binary into the local directory.
```shell
go build
```
## Usage

See help:
```shell
faas
```

## Configuration

### Cluster Prerequisites

see https://github.com/lkingland/config for cluster setup and configuration.  Broadly, requirements are:
* Kubernetes
* Knative Serving and Eventing
* Knative Domains patched to enable domains
* Knative Network patched to enable subdomains
* Kourier

### Container Registry

Both the image registry and user/org namespace need to be defined either by
using the --registry and --namespace flags on the `create` command, or by
configuring as environment variables.  For example to configure all images
to be pushed to `quay.io/alice`, use:
```
export FAAS_REGISTRY=quay.io
export FAAS_NAMESPACE=alice
```

## Examples

Create a new Function Service:

```shell
> mkdir -p example.com/www
> cd example.com/www
> faas create go
OK www.example.com
> curl https://www.example.com
OK
```


