# Building Behind a TLS-Intercepting Proxy (Custom CA)

Some networks send all HTTPS traffic through a proxy that re-signs it with a
corporate CA. A build then fails to download its dependencies with errors like
`x509: certificate signed by unknown authority`, because the build container
does not trust that CA.

`--build-cafile` makes the build trust it:

```
func build --build-cafile ~/corp-ca.pem
```

The file is a PEM file with one or more CA certificates. A relative path is
resolved against the function directory. The certificates are used at build
time only and are not added to the function image.

The flag is supported by `func build`, `func deploy` and `func run`, with the
pack builder only. It can also be set with `FUNC_BUILD_CAFILE`.

## Where the setting is kept

The path describes your machine, not the function, so it is remembered in the
function's local settings, `.func/local.yaml`, and not in `func.yaml`. The
`.func` directory is not committed, so other copies of the function (a
colleague's checkout, CI, an on-cluster rebuild) are not affected.

Builds that do not support it ignore the remembered value: a later
`func deploy --remote` or a build with another builder works as usual. Giving
`--build-cafile` (or `FUNC_BUILD_CAFILE`) on such a command is an error.

To stop using it, pass an empty value once: `func build --build-cafile=`.

## How it works

The certificates are given to the build as a service binding of type
`ca-certificates`, mounted read-only at
`/platform/bindings/func-build-ca` (or under `$SERVICE_BINDING_ROOT` when you
set it as a build env). The
[Paketo CA Certificates buildpack](https://github.com/paketo-buildpacks/ca-certificates)
adds them to the trust store of the build, one certificate per file
(`ca-0.pem`, `ca-1.pem`, ...). On SELinux hosts (e.g. Podman on Fedora/RHEL)
the binding is relabeled so the build can read it.

## Runtimes

Checked with the default builders behind a TLS-intercepting proxy:

| Runtime | Works with `--build-cafile` alone |
|---|---|
| Go, Node.js, TypeScript, Quarkus, Spring Boot | yes |
| Python | no, see below |
| Rust | no, see below |

### Python

pip trusts the CA, but Poetry, which installs the function's dependencies, uses
its own CA bundle and ignores the one Paketo adds to. Point it at the mounted
certificate with the `REQUESTS_CA_BUNDLE` build env in `func.yaml`:

```yaml
build:
  buildEnvs:
    - name: REQUESTS_CA_BUNDLE
      value: /platform/bindings/func-build-ca/ca-0.pem
```

The same caveats apply as for `GIT_SSL_CAINFO` below: it replaces the whole
bundle, and it is committed with `func.yaml`.

### Rust

The Rust buildpacks of the default builder do not include the CA Certificates
buildpack, so the certificate is not applied and the build cannot download its
toolchain. There is no workaround yet.

## git

git in the build image is linked against GnuTLS and reads only its own CA
bundle, so it does not trust the CA on its own. Dependencies fetched with git,
such as Go modules from a VCS (`GOPROXY=direct`, private modules), then fail
with `server certificate verification failed`.

Point git at the mounted certificate with the `GIT_SSL_CAINFO` build env in
`func.yaml`:

```yaml
build:
  buildEnvs:
    - name: GIT_SSL_CAINFO
      value: /platform/bindings/func-build-ca/ca-0.pem
```

Keep in mind:

- `GIT_SSL_CAINFO` replaces git's whole CA bundle, so git then trusts only that
  certificate. This fits a TLS-intercepting proxy, which re-signs every
  connection with its CA.
- Unlike `--build-cafile`, build envs are part of `func.yaml` and get
  committed. A build without `--build-cafile` has no such file, and git then
  fails, so only keep this on a copy that is always built behind the proxy.
- If your file holds several certificates, pick the `ca-N.pem` of the one git
  needs (they keep the order of your file).
