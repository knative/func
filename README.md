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
## Examples
```shell
> mkdir -p example.com/www
> cd example.com/www
> faas create go
OK www.example.com
> curl https://www.example.com
OK
```


