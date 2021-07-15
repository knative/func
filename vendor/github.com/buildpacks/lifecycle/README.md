# Lifecycle

![Build Status](https://github.com/buildpacks/lifecycle/workflows/build/badge.svg)
[![GoDoc](https://godoc.org/github.com/buildpacks/lifecycle?status.svg)](https://godoc.org/github.com/buildpacks/lifecycle)

A reference implementation of the [Cloud Native Buildpacks specification](https://github.com/buildpacks/spec).

## Supported APIs
Lifecycle Version | Platform APIs                            | Buildpack APIs |
------------------|------------------------------------------|----------------|
0.10.x            | [0.3][p/0.3], [0.4][p/0.4], [0.5][b/0.5] | [0.2][b/0.2], [0.3][b/0.3], [0.4][b/0.4], [0.5][b/0.5]
0.9.x             | [0.3][p/0.3], [0.4][p/0.4]               | [0.2][b/0.2], [0.3][b/0.3], [0.4][b/0.4]
0.8.x             | [0.3][p/0.3]                             | [0.2][b/0.2]
0.7.x             | [0.2][p/0.2]                             | [0.2][b/0.2]
0.6.x             | [0.2][p/0.2]                             | [0.2][b/0.2]

[b/0.2]: https://github.com/buildpacks/spec/blob/buildpack/v0.2/buildpack.md
[b/0.3]: https://github.com/buildpacks/spec/tree/buildpack/v0.3/buildpack.md
[b/0.4]: https://github.com/buildpacks/spec/tree/buildpack/v0.4/buildpack.md
[b/0.5]: https://github.com/buildpacks/spec/tree/buildpack/v0.5/buildpack.md
[p/0.2]: https://github.com/buildpacks/spec/blob/platform/v0.2/platform.md
[p/0.3]: https://github.com/buildpacks/spec/blob/platform/v0.3/platform.md
[p/0.4]: https://github.com/buildpacks/spec/blob/platform/v0.4/platform.md
[p/0.5]: https://github.com/buildpacks/spec/blob/platform/v0.5/platform.md

## Commands

### Build

Either:
* `detector` - Chooses buildpacks (via `/bin/detect`) and produces a build plan.
* `analyzer` - Restores layer metadata from the previous image and from the cache.
* `restorer` - Restores cached layers.
* `builder` -  Executes buildpacks (via `/bin/build`).
* `exporter` - Creates an image and caches layers.

Or:
* `creator` - Runs the five phases listed above in order.

### Run

* `launcher` - Invokes a chosen process.

### Rebase

* `rebaser` - Creates an image from a previous image with updated base layers.

## Development
To test, build, and package binaries into an archive, simply run:

```bash
$ make all
```
This will create an archive at `out/lifecycle-<LIFECYCLE_VERSION>+linux.x86-64.tgz`.

`LIFECYCLE_VERSION` defaults to the value returned by `git describe --tags` if not on a release branch (for more information about the release process, see [RELEASE](RELEASE.md). It can be changed by prepending `LIFECYCLE_VERSION=<some version>` to the
`make` command. For example:

```bash
$ LIFECYCLE_VERSION=1.2.3 make all
```

Steps can also be run individually as shown below.

### Test

Formats, vets, and tests the code.

```bash
$ make test
```

### Build

Builds binaries to `out/linux/lifecycle/`.

```bash
$ make build
```

> To clean the `out/` directory, run `make clean`.

### Package

Creates an archive at `out/lifecycle-<LIFECYCLE_VERSION>+linux.x86-64.tgz`, using the contents of the
`out/linux/lifecycle/` directory, for the given (or default) `LIFECYCLE_VERSION`.

```bash
$ make package
```
