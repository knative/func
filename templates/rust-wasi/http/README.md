# Rust WASI HTTP Function

Welcome to your new Rust WASI function! The handler is in
[`src/lib.rs`](./src/lib.rs). It implements the `wasi:http/incoming-handler`
interface from the [WASI HTTP](https://github.com/WebAssembly/wasi-http) spec
and targets the `wasm32-wasip2` (WASI Preview 2) platform.

## Prerequisites

- [Rust](https://rustup.rs/) toolchain
- `wasm32-wasip2` target:

  ```shell
  rustup target add wasm32-wasip2
  ```

## Development

Put your function logic inside the `process` function in `src/lib.rs`. The
`Guest::handle` method is kept thin on purpose: `IncomingRequest` is a
host-owned WIT resource with no guest-side constructor, so business logic lives
in `process` where it can be unit-tested with plain Rust values.

Run unit tests with the native toolchain (no WASI host required):

```shell
cargo test
```

Build the WASM module locally to verify it compiles:

```shell
cargo build --target wasm32-wasip2 --release
```

The output is at `target/wasm32-wasip2/release/function.wasm`.

## Deployment

Use `func` to build and deploy the function:

```shell
func deploy --registry=docker.io/<YOUR_ACCOUNT>
```

The `func` CLI automatically uses the `wasm` builder for `rust-wasi` functions.

## Invocation

Once deployed, invoke the function with `curl`:

```console
curl $(func info -o url)
```

Have fun!
