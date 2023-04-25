# Lifecycle Hooks

Function runtimes should provide the function developer an ability to perform basic lifecycle operations on the function.
In addition to the default `/` endpoints that all function runtimes expose, there should also be a set of lifecycle endpoints
that allow function developers to execute code at startup and shutdown. Additionally, function runtimes should provide
the function developer an ability to override the implementation and routing of the HTTP health endpoints at `/health/liveness`
and `/health/readiness`.

## Hook APIs

Due to the fact that each function runtime is implemented in a different language, the APIs for the lifecycle hooks are not
consistent across all runtimes. This document describes the intent of the lifecycle hooks and the expected behavior of the
function runtime, but does not address the implementation details for each language. Currently none of the function runtimes
expose this behavior. This document is forward-looking.

Function lifecycle hooks allow the function developer to provide custom code that is executed at specific points in the
function's lifecycle. The function developer can provide a function that is executed when the function is started, and when the
function is stopped. This allows the function developer to perform any initialization or cleanup tasks that are required
for the function to operate correctly.

To provide custom code for the lifecycle hooks, runtime implementations should NOT require modification to the function
configuration in the `func.yaml` file. Instead, the function developer should be able to provide the code for the lifecycle
hooks in the function source code itself. As previously mentioned, the details of these implementations are language-specific.

### Init

When a function is first started, the runtime should execute the function's `init` function, if provided by the function
developer. The `init` function is executed before the function is ready to receive requests. The `init` function is
expected to be synchronous, and should return a boolean value indicating whether the function was successfully initialized.
The `init` function should not accept any arguments.

### Shutdown

When a function is stopped, the runtime should execute the function's `shutdown` function, if provided by the function
developer. The `shutdown` function is executed after the function has stopped receiving requests, but before the function
process has been stopped. The `shutdown` function is expected to be synchronous, and should return a boolean value
indicating whether the function was successfully shutdown. The `shutdown` function should not accept any arguments.

## Health Endpoints

Out of the box, the function runtimes provide a default implementation of the HTTP health endpoints. These endpoints
are used by Knative to determine if the function is ready to receive requests. The default implementation of the health
endpoints varies by language, but in general, the `/health/liveness` endpoint is used to determine if the function is
running, and the `/health/readiness` endpoint is used to determine if the function is ready to receive requests.

Function runtimes should provide the function developer an ability to override the implementation of the HTTP health endpoints.
By default, functions are expected to respond to HTTP requests on the `/health/liveness` and `/health/readiness` endpoints.
Language Packs may change these values by providing a `healthEndpoints` section in the `manifest.yaml` file.

However, this alone does not allow the function developer to override the default implementation of the health endpoints.
Function developers should be able to provide custom code for the health endpoints in order to provide custom logic for
determining if the function is ready to receive requests. This is useful for functions that require some sort of
initialization before they are ready to receive requests or which may need to determine if some external service is
available before they are ready to receive requests.

To provide custom code for the health endpoints, runtime implementations should NOT require modification to the function
configuration in the `func.yaml` file. Instead, the function developer should be able to provide the code for the health
endpoints in the function source code itself. As previously mentioned, the details of these implementations are language-specific.

Health endpoints are expected to be synchronous and should result in an HTTP `200 OK` response code if the function is
ready to receive requests. If the function is not ready to receive requests, the health endpoint should result in
`503 Service Unavailable`.
