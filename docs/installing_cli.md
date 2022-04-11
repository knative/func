# Installing the CLI

The CLI is used to create, build, manage and deploy Functions.  There are a few different ways you can install the binary.

## From Homebrew

```
brew tap knative-sandbox/kn-plugins
brew install kn
brew install func
```

Installed in this way, Knative Functions are managed as a plugin of the Knative CLI, via `kn func`. You may also invoke the Functions client directly from the Homebrew path, often `/opt/homebrew/bin`, as `kn-func`.

## Prebuilt Binary

Download the latest binary appropriate for your system from the [Latest Release](https://github.com/knative-sandbox/kn-plugin-func/releases/latest/).

Each version is built and made available as a prebuilt binary.  See [All Releases](https://github.com/knative-sandbox/kn-plugin-func/releases/).

## From Source

To build and install from source check out the repository, run `make`, and install the resultant binary:
```
git clone git@github.com:knative-sandbox/kn-plugin-func.git
cd kn-plugin-func
make
sudo mv func /usr/local/bin/
```

## Integration with `kn`
See [plugins](https://github.com/knative/client/blob/main/docs/README.md#plugin-configuration) section of `kn`.
