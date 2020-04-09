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


### Configuration

A public repository is presently required for the intermediate container.  As such, one's local
docker (or podman) should be logged in, and an environment variable set to the user or organization
name to which images should be deployed:
```
export FAAS_NAMESPACE=johndoe
```

### Examples

Create a new Function Service:

```shell
> mkdir -p example.com/www
> cd example.com/www
> faas create go
OK www.example.com
> curl https://www.example.com
OK
```


