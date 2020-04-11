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

### Knative Serving Network Configuraiton

Patch the Knative Network Config to enable subdomains:
```
kubectl apply -f ./k8s/config-network.yaml`
```

Patch the Knative Domains Config to set a default domain:
```
kubectl apply -f ./k8s/config-domain.yaml`
```

### Public Container Registry and Namespace

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


