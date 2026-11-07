# The func-utils image

`ghcr.io/knative/func-utils` holds `cmd/func-util`, which func runs in the
cluster: Tekton build steps, the socat dialer and the volume upload. Each
func CLI embeds one tag of it.

## Tags

| Tag | Used by |
| --- | --- |
| `latest` | CLIs before 2025. Frozen. |
| `v2` | CLIs up to 1.23. Frozen. |
| `X.Y` | CLIs `vX.Y.*`. Published from `release-X.Y`, and from `main` while `X.Y` is in development. |

Ideally we would not backport to `release-X.Y` a change that breaks how the CLI
calls the image: arguments, exit codes, parsed output or shared files.
In most cases the latest `X.Y` image will work with every `vX.Y.*` CLI.

## Where the tag lives

`FuncUtilsImage` in `pkg/version/version.go`. `make func-utils-image`
prints what a build embeds. To embed another image, set `FUNC_UTILS_IMG`.

## Release cut

`main` embeds the minor in development, say `1.24`, and every green push
publishes `:1.24`. When `release-1.24` is cut:

1. The branch already embeds `:1.24`, and the image exists, so the
   release can go. `hack/release.sh` refuses a `v1.24.z` release
   otherwise.
2. `.github/workflows/update-func-utils-image.yaml` opens a PR that moves
   `main` to `1.25`. Until it merges, `main` stops publishing `:1.24`,
   except for the cut commit itself.
3. Every green push to `release-1.24` (a backport) publishes `:1.24`.

If `:1.24` holds the wrong image, re-run the Functions run for the
newest commit on `release-1.24`.

If `main` still embeds `1.24` when `release-1.25` is cut, set
`FuncUtilsImage` to `ghcr.io/knative/func-utils:1.25` on `release-1.25`
and to `:1.26` on `main`.
