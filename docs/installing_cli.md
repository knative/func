# Installing the CLI

The CLI can be used to invoke most features of the FaaS system.  One can choose to run the container, install one of the pre-built binaries, or compile from source.

### Container

The latest release can be run as a container:
```
docker run quay.io/boson/faas
```
To run a specific version of the CLI, use the version desired as the image tag:
```
docker run quay.io/boson/faas:v0.5.0
```

### Prebuilt Binary

Download the latest binary appropriate for your system from the [Latest Release](https://github.com/boson-project/faas/releases/latest/).

Each version is built and made available as a prebuilt binary.  See [All Releases](https://github.com/boson-project/faas/releases/).

### From Source


To build and install from source check out the repository, run `make`, and install the resultant binary:
```
make
mv faas /usr/local/bin/
```
