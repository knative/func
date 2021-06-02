# Project Configuration with `func.yaml`

The `func.yaml` file contains configuration information for your function
project. Generally, these values are used when you execute a `func` CLI
command. For example, when `func build` is run, the CLI uses the value for
the `builder` field. In many cases, these values may be overridden by
command line flags or environment variables. For more information about
overriding these values, consult the [Commands](command.md) document.

Many of the fields are generated for you when you create, build and deploy
your function. However there are a few that you may use to tweak things
such as the function name, and the image name.

## Fields

The following fields are used in `func.yaml`.

### `builder`

Specifies the buildpack builder image to use when building the function.
In most cases, this value should not be changed.

### `builderMap`

Some function runtimes may be built in multiple ways. For example, a Quarkus
function may be built for the JVM, or as a native binary. The `builderMap`
field will contain all of the available builders for a given runtime. Although
it's typically unnecessary to modify the `builder` field, using values from
`builderMap` is OK.

### `envs`

The `envs` field allows you to set environment variables that will be
available to your function at runtime. 
1. Environment variable can be set directly from a value
2. Environment variable can be set from a local environment value. Eg. `'{{ env.LOCAL_ENV_VALUE }}'`, for more details see [Local Environment Variables section](#local-environment-variables).
3. Environment variable can be set from a key in a Kubernetes Secret. This Secret needs to be created before it is referenced in a function. Eg. `'{{ secret.mysecret.key }}'` where `mysecret` is the name of the Secret and `key` is the referenced key.
4. All key-value pairs from a Kubernetes Secret will be set as environment variables. This Secret needs to be created before it is referenced in a function. Eg. `'{{ secret.mysecret2 }}'` where `mysecret2` is the name of the Secret.

```yaml
envs:
- name: EXAMPLE1                       # (1) env variable directly from a value
  value: value
- name: EXAMPLE2                       # (2) env variable from a local environment value
  value: '{{ env.LOCAL_ENV_VALUE }}'
- name: EXAMPLE3                       # (3) env variable from a key in Secret
  value: '{{ secret.mysecret.key }}'
- value: '{{ secret.mysecret2 }}'      # (4) all key-value pairs in Secret as env variables
```

### `volumes`
Kubernetes Secrets can be mounted to the function as a Kubernetes Volume accessible under specified path. Below you can see an example how to mount the Secret `mysecret` to the path `/workspace/secret`. This Secret needs to be created before it is referenced in a function.

```yaml
volumes:
- secret: mysecret
  path: /workspace/secret
```

### `image`

This is the image name for your function after it has been built. This field
may be modified and `func` will create your image with the new name the next
time you run `kn func build` or `kn func deploy`.

### `imageDigest`

This is the `sha256` hash of the image manifest when it is deployed. This value
should not be modified.

### `name`

The name of your function. This value will be used as the name for your service
when it is deployed. This value may be changed to rename the function on
subsequent deployments.

### `namespace`

The Kubernetes namespace where your function will be deployed.

### `runtime`

The language runtime for your function. For example `python`.

### `trigger`

The invocation event that triggers your function. Possible values are `http`
for plain HTTP requests, and `events` for CloudEvent triggered functions.


## Local Environment Variables

Any of the fields in `func.yaml` may contain a reference to an environment
variable available in the local environment. For example, if I would like
to avoid storing sensitive information such as an API key in my function
configuration, I may have this value set from the local environment. To do
this, prefix the local environment variable with `{{` and `}}` and prefix
the name with `env.`. For example:

```yaml
envs:
- name: API_KEY
  value: '{{ env.API_KEY }}'
```
