# Function project

Welcome to your new Quarkus function project!

This sample project contains single function: `functions.Function.echo()`,
the function just returns its argument.

## Local execution
Make sure that `Java 11 SDK` is installed.

To start server locally run `./mvnw quarkus:dev`.
The command starts http server and automatically watches for changes of source code.
If source code changes the change will be propagated to running server. It also opens debugging port `5005`
so debugger can be attached if needed.

To run test locally run `./mvnw test`.

## The `function` CLI

It's recommended to set `FUNCTION_REGISTRY` environment variable.
```shell script
# replace ~/.bashrc by your shell rc file
# replace docker.io/johndoe with your registry
export FUNCTION_REGISTRY=docker.io/johndoe
echo "export FUNCTION_REGISTRY=docker.io/johndoe" >> ~/.bashrc 
```

### Building

This command builds OCI image for the function.

```shell script
function build                  # build jar
function build --builder native # build native binary
```

### Running

This command runs the function locally in a container
using the image created above.
```shell script
function run
```

### Deploying

This commands will build and deploy the function into cluster.

```shell script
function deploy # also triggers build
```

## Function invocation

Do not forget to set `URL` variable to the route of your function.

You get the route by following command.
```shell script
function describe
```

### cURL

```shell script
URL=http://localhost:8080/
curl -v ${URL} \
  -H "Content-Type:application/json" \
  -H "Ce-Id:1" \
  -H "Ce-Source:cloud-event-example" \
  -H "Ce-Type:dev.knative.example" \
  -H "Ce-Specversion:1.0" \
  -d "{\"name\": \"$(whoami)\"}\""
```

### HTTPie

```shell script
URL=http://localhost:8080/
http -v ${URL} \
  Content-Type:application/json \
  Ce-Id:1 \
  Ce-Source:cloud-event-example \
  Ce-Type:dev.knative.example \
  Ce-Specversion:1.0 \
  name=$(whoami)
```
