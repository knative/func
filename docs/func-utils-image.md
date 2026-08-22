# The func-utils image

`ghcr.io/knative/func-utils` packages `cmd/func-util`, a single Go binary
that dispatches on its invocation name (`deploy`, `scaffold`, `s2i`,
`s2i-generate`, `socat`, `sh`). The `func` CLI does not run this image
itself; it creates pods and Tekton TaskRuns from it: Tekton build steps,
the socat dialer, and the tar-based volume upload.

The consumers are already-shipped CLI binaries. Each released CLI embeds
one image reference at build time (see below) and then invokes the image
with fixed argv shapes, parses fixed stderr strings, and shares files such
as `middleware-version` with it. This interface is an ABI between old
binaries and a mutable tag: changing the image under a tag changes
behavior for every CLI that embeds that tag, including ones released long
ago.

## Tag scheme

| Tag | Meaning |
| --- | --- |
| `latest` | Frozen legacy tag for pre-2025 CLIs. Never moves. |
| `v2` | Floating tag. Published from every green push to `main`. Embedded by dev, nightly, and main builds. |
| `X.Y` (for example `1.24`) | Per-minor pinned tag. Published from every green push to the `release-X.Y` branch. Embedded by all released `vX.Y.*` CLIs. |

Backports merged to a `release-X.Y` branch re-publish `X.Y`, so fixes
still reach every CLI of that minor. Pushes to `main` never touch `X.Y`,
so main development cannot break released CLIs.

The per-minor tags carry no `v` prefix. This keeps them in a separate
namespace from the contract-version tags `v1`/`v2`, so the two schemes
cannot collide.

## Compatibility rule

Within any published tag, the sub-command contracts are frozen: argv
shapes, exit codes, parsed stderr strings, and shared files.

- A change that breaks any of these must never be backported to a
  `release-X.Y` branch.
- On `main`, such a change requires bumping the floating tag (`v2` to
  `v3`) in the same PR as the new source defaults, so older embedded
  references keep resolving to a compatible image.

Motivation: [#2686](https://github.com/knative/func/pull/2686) (the
image became incompatible with shipped CLIs, so `latest` was frozen and
`v2` introduced in the same change) and
[#3237](https://github.com/knative/func/pull/3237) (`scaffold` grew a
third argument with no tag bump, silently breaking on-cluster builds for
every older released CLI).

## Build-time derivation

The Makefile derives `FUNC_UTILS_IMG` and embeds it into three variables
(`pkg/k8s.SocatImage`, `pkg/k8s.TarImage`,
`pkg/pipelines/tekton.FuncUtilImage`) via ldflags. Release builds (an
exact release `KVER`, or a `release-X.Y` branch checkout) pin to `X.Y`;
everything else floats on `v2`. An explicitly provided `FUNC_UTILS_IMG`
always wins.

`make func-utils-image` prints the effective reference. `hack/images.sh`
uses it to push the locally built image under the same tag for e2e tests.

## Release-cut checklist

After cutting a `release-X.Y` branch:

1. Verify the Functions workflow run for the branch push succeeded.
2. Verify the tag exists:
   `docker manifest inspect ghcr.io/knative/func-utils:X.Y`.
3. If either failed, fix and re-push, or trigger the Functions workflow
   manually (`workflow_dispatch`) on the branch.

The `X.Y` tag must exist before the `vX.Y.0` release is published;
otherwise the released CLI embeds a reference that does not resolve.
