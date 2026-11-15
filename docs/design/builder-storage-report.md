# Builder storage reporting proposal

Status: proposal for [knative/func#4047](https://github.com/knative/func/issues/4047)

## Problem

Local builds can leave builder images, function images, daemon build cache, and
function-local cache data on disk. `func` currently gives users no inventory
that helps explain this storage use. Cleanup is risky because some images and
cache records may be shared with other projects or tools.

## Current storage paths

- The `pack` and `s2i` builders build through a Docker-compatible daemon.
- The `host` OCI builder keeps downloaded base-image layers under the current
  function's `.func/build/blob-cache` directory.
- Builder images can be defaults or user-supplied through `func.yaml` and
  `--builder-image`, so registry or name prefixes alone cannot identify every
  image that belongs to `func`.
- Buildpack cache ownership is delegated to Pack and the container engine.
  `func` has no shared registry of which local projects currently reference an
  image or cache entry.

## Proposed first step: report only

Add a `func builders` command that reports storage the selected container
engine exposes. Use the existing Docker-compatible client so the report works
with supported Docker and Podman connections, including remote daemon
connections.

The report should show image and build-cache entries with the engine-reported
size and reclaimable size. It should identify known default builder images and
leave custom or unrecognized images as unknown. Engine totals should be shown
separately from per-image sizes because image layers may be shared and must not
be added together as unique disk use.

When `--path` is supplied, also report the size of that function's
`.func/build/blob-cache`. Do not scan the user's home directory or other
function directories. A path-local report can name the function's configured
builder image without making claims about references in other projects.

If the selected engine cannot provide image or cache usage details, report the
unsupported category clearly and continue with the categories it can provide.
If the engine is unavailable, return an actionable connection error rather
than an empty successful report.

The initial output can be a human-readable table. Machine-readable output and
build-time hit/miss reporting should be considered separately after the
inventory contract is reviewed. Pack and OCI produce cache information through
different implementations, so a `func build --cache-report` flag should not be
promised by this first step.

## Follow-up: pruning

Pruning should be a separate contribution after reporting establishes the
available storage metadata. It must only remove entries the engine identifies
as reclaimable or entries that `func` can prove it owns. The command should
print a dry-run plan by default and require explicit confirmation before
deletion. It must not infer that a custom image or cache is unused from its
name, age, or the contents of the current function directory.

## Implementation sequence

1. Add a small storage-report interface and a Docker-compatible implementation
   for image and build-cache usage.
2. Add the `func builders` CLI output with unit tests using a fake report
   provider. Cover shared image sizes, unavailable daemon data, and engine
   connection errors.
3. Add path-scoped measurement of `.func/build/blob-cache`, with tests for an
   empty cache and nested files.
4. Consider JSON output and build-time cache hit/miss reporting separately.
5. Design pruning only after the report's ownership and reclaimable fields are
   validated against supported Docker and Podman versions.

## Open questions

- Which Docker and Podman API versions provide sufficiently consistent
  per-image and build-cache size data?
- Should the first command be named `func builders`, or should it live under a
  broader `func cache` command if future cache sources are expected?
- Should the first report include only daemon storage and the optional
  function-local OCI cache, or also display builder-image metadata from Pack?
- What minimum daemon support level should be required before offering a prune
  command?
