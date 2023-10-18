# Function project

Welcome to your new Function project!

This sample project contains a single function based on Spring Cloud Function: `functions.CloudFunctionApplication.echo()`, which returns an echo of the data passed via HTTP request.

## Local execution

Make sure that `Java 17 SDK` is installed since Spring Boot 3.0 requires Java 17.

To start server locally run `./mvnw spring-boot:run`.
The command starts http server and automatically watches for changes of source code.
If source code changes the change will be propagated to running server. It also opens debugging port `5005`
so a debugger can be attached if needed.

To run tests locally run `./mvnw test`.

## The `func` CLI

It's recommended to set `FUNC_REGISTRY` environment variable.

```shell script
# replace ~/.bashrc by your shell rc file
# replace docker.io/johndoe with your registry
export FUNC_REGISTRY=docker.io/johndoe
echo "export FUNC_REGISTRY=docker.io/johndoe" >> ~/.bashrc
```

### Building

This command builds an OCI image for the function. By default, this will build a JVM image.

```shell script
func build -v                  # build image
```

**Note**: If you want to enable the native build, you need to edit the `func.yaml` file and
set the `BP_NATIVE_IMAGE` BuilderEnv variable to true:

```yaml
buildEnvs:
  - name: BP_NATIVE_IMAGE
    value: "true"
```

**Note**: If you have issues with the [Spring AOT](https://docs.spring.io/spring-framework/docs/current/reference/html/core.html#core.aot) processing in your build, you can turn this off by editing the `func.yaml` file and removing the `BP_MAVEN_ACTIVE_PROFILES` buildEnvs entry.

> Removing the `native` profile means that you no longer will be able to build as a native image.

### Running

This command runs the func locally in a container
using the image created above.

```shell script
func run
```

### Deploying

This command will build and deploy the function into cluster.

```shell script
func deploy -v # also triggers build
```

### For ARM processor based systems

Building Spring Boot apps with Paketo Buildpacks on an ARM processor based system, like an Apple Macbook with an M1 or M2 chip, is not fully supported at the moment.
There is work in progress to fix this, you can follow the [Support ARM #435](https://github.com/buildpacks/lifecycle/issues/435) issue.
You can also join the `#arm` channel in the paketo slack (join at: https://slack.paketo.io/) to follow progress on this topic

To work around the multiarch limitation you can build the image on your local system using a community contributed [multiarch builder](https://github.com/dashaun/paketo-arm64) that produces images for the `arm64` arch.

You need to provide the `--builder-image` argument using `dashaun/builder:tiny` as the builder.

```sh
func build -v --builder-image dashaun/builder:tiny
func deploy --build=false
```

> Note: This assumes that you are targeting a cluster that supports arm64 based images
> which would be the case if you ran a kind cluster on your local Mac system.

## Function invocation

For the examples below, please be sure to set the `URL` variable to the route of your function.

You get the route by following command.

```shell script
func info
```

Note the value of **Routes:** from the output, set `$URL` to its value.

__TIP__:

If you use `kn` then you can set the url by:

```shell script
# kn service describe <function name> and show route url
export URL=$(kn service describe $(basename $PWD) -ourl)
```

### func

Using `func invoke` command with Path-Based routing:

```shell script
func invoke --target "$URL/echo" --data "$(whoami)"
```

If your function class only contains one function, then you can leave out the target path:

```shell script
func invoke --data "$(whoami)"
```

### cURL

```shell script
curl -v "$URL/echo" \
  -H "Content-Type:text/plain" \
  -w "\n" \
  -d "$(whoami)"
```

If your function class only contains one function, then you can leave out the target path:

```shell script
curl -v "$URL" \
  -H "Content-Type:text/plain" \
  -w "\n" \
  -d "$(whoami)"
```

### HTTPie

```shell script
echo "$(whoami)" | http -v "$URL/echo"
```

If your function class only contains one function, then you can leave out the target path:

```shell script
echo "$(whoami)" | http -v "$URL"
```

## Cleanup

To clean the deployed function run:

```shell
func delete
```
