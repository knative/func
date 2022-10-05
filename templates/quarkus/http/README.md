# Function project

Welcome to your new Quarkus function project!

This sample project contains a single function: `functions.Function.function()`,
the function just returns its argument.

## Local execution
Make sure that `Java 11 SDK` is installed.

To start server locally run `./mvnw quarkus:dev`.
The command starts http server and automatically watches for changes of source code.
If source code changes the change will be propagated to running server. It also opens debugging port `5005`
so debugger can be attached if needed.

To run test locally run `./mvnw test`.

## The `func` CLI

It's recommended to set `FUNC_REGISTRY` environment variable.
```shell script
# replace ~/.bashrc by your shell rc file
# replace docker.io/johndoe with your registry
export FUNC_REGISTRY=docker.io/johndoe
echo "export FUNC_REGISTRY=docker.io/johndoe" >> ~/.bashrc 
```

### Building

This command builds OCI image for the function.

```shell script
func build
```

By default, JVM build is used.
To enable native build set following environment variables to `func.yaml`:
```yaml
buildEnvs:
- name: BP_NATIVE_IMAGE
  value: "true"
- name: BP_MAVEN_BUILT_ARTIFACT
  value: func.yaml target/native-sources/*
- name: BP_MAVEN_BUILD_ARGUMENTS
  value: package -DskipTests=true -Dmaven.javadoc.skip=true -Dquarkus.package.type=native-sources
- name: BP_NATIVE_IMAGE_BUILD_ARGUMENTS_FILE
  value: native-image.args
- name: BP_NATIVE_IMAGE_BUILT_ARTIFACT
  value: '*-runner.jar'

```

### Running

This command runs the func locally in a container
using the image created above.
```shell script
func run
```

### Deploying

This commands will build and deploy the function into cluster.

```shell script
func deploy # also triggers build
```

## Function invocation

Do not forget to set `URL` variable to the route of your function.

You get the route by following command.
```shell script
func info
```

### cURL

```shell script
URL=http://localhost:8080/
curl -v ${URL} \
  -H "Content-Type:application/json" \
  -d "{\"message\": \"$(whoami)\"}\""
# OR
URL="http://localhost:8080/?message=$(whoami)"
curl -v ${URL} 
```

### HTTPie

```shell script
URL=http://localhost:8080/
http -v ${URL} \
  message=$(whoami)
# OR
URL="http://localhost:8080/?message=$(whoami)"
http -v ${URL}
```
