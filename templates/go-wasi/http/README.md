# Go WASI HTTP Function

Welcome to your new Go WASI function! The handler is in
[`function.go`](./function.go). It uses the standard `net/http` package via
[TinyGo](https://tinygo.org/)'s WASI HTTP support and targets the `wasip2`
platform.

## Prerequisites

- [TinyGo](https://tinygo.org/getting-started/install/) 0.33+

## Development

Put your function logic inside `handle` in `function.go`. The handler is a
plain `http.HandlerFunc`, so you can test it with `net/http/httptest` using the
standard Go toolchain — no WASI host required.

Run unit tests:

```shell
go test ./...
```

Build the WASM module locally to verify it compiles:

```shell
tinygo build -target=wasip2 -o function.wasm .
```

## Deployment

Use `func` to build and deploy the function:

```shell
func deploy --registry=docker.io/<YOUR_ACCOUNT>
```

The `func` CLI automatically uses the `wasm` builder for `go-wasi` functions.

## Invocation

Once deployed, invoke the function with `curl`:

```console
curl $(func info -o url)
```

Have fun!
