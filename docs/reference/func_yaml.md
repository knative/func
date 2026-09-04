# Project Configuration with `func.yaml`

The `func.yaml` file contains configuration information for your function
project. Generally, these values are used when you execute a `func` CLI
command. For example, when `func build` is run, the CLI uses the value for
the `builder` field. In some cases, these values may be overridden by
command line flags or environment variables. For more information about
overriding these values, consult the [Commands](func.md) document.

Many of the fields are generated for you when you create, build and deploy
your function. Generally, function developers do not need to manually edit
this file. However there are a few that you may use to tweak things
such as the function name, and the image name.

## Fields

The following fields are used in `func.yaml`.


### `build`

Specifies how to build the function. Possible values are "local" to build on your local
computer, or "git" to build on the cluster by pulling function source code from a git
repository.

### `builderImages`

Defines the builder images to use by builder implementations in lieu of the defaults.
They key is the builder's short name.  For example:

```
build:
  builderImages:
	  pack: example.com/user/my-pack-node-builder
    s2i: example.com/user/my-s2i-node-builder
```

### `deployer`

The type of deployment to use when deploying the function. Possible values are:
- `knative` (default): deploys a Knative Service, scaled by Knative's KPA (Knative Pod Autoscaler).
- `raw`: deploys a plain Kubernetes Deployment with a static replica count.
- `keda`: deploys a plain Kubernetes Deployment scaled by [KEDA](https://keda.sh), based on triggers such as incoming HTTP traffic or Kafka consumer lag. See [`options.scale.keda`](#options) below.

```yaml
deployer: keda
```

### `git`

If using a `git` build strategy, this field is used to specify the git URL as well
as an optional context directory. For example:

```
git:
  url: github.com/boson-project/example
  contextDir: subdirectory
```

### `buildEnvs`
This field allows you to set environment variables available to the builder/buildpack that builds the function. This environment variable is NOT set at runtime, use [envs](#envs) instead
1. Environment variable can be set directly from a value
2. Environment variable can be set from a local environment value. Eg. `'{{ env:LOCAL_ENV_VALUE }}'`, for more details see [Local Environment Variables section](#local-environment-variables).

```yaml
buildEnvs:
- name: EXAMPLE1                            # (1) env variable directly from a value
  value: value
- name: EXAMPLE2                            # (2) env variable from a local environment value
  value: '{{ env:LOCAL_ENV_VALUE }}'
```

For example, the below `func.yaml` snippet modifies the default Golang buildpack to build source code with 1.15 compiler version. Refer to respective buildpack documentation to know more about environment variables that modify behavior of the `func build`.
```yaml
buildEnvs:
- name: BP_GO_VERSION
  value: '1.15'
```

### `envs`

The `envs` field allows you to set environment variables that will be
available to your function at runtime.
1. Environment variable can be set directly from a value
2. Environment variable can be set from a local environment value. Eg. `'{{ env:LOCAL_ENV_VALUE }}'`, for more details see [Local Environment Variables section](#local-environment-variables).
3. Environment variable can be set from a key in a Kubernetes Secret or ConfigMap. This Secret/ConfigMap needs to be created before it is referenced in a function. Eg. `'{{ secret:mysecret:key }}'` where `mysecret` is the name of the Secret and `key` is the referenced key; or `{{ configMap:myconfigmap:key }}` where `myconfigmap` is the name of the ConfigMap and `key` is the referenced key.
4. All key-value pairs from a Kubernetes Secret or ConfigMap will be set as environment variables. This Secret/ConfigMap needs to be created before it is referenced in a function. Eg. `'{{ secret:mysecret2 }}'` where `mysecret2` is the name of the Secret: or `{{ configMap:myconfigmap }}` where `myconfigmap` is the name of the ConfigMap.

```yaml
envs:
- name: EXAMPLE1                            # (1) env variable directly from a value
  value: value
- name: EXAMPLE2                            # (2) env variable from a local environment value
  value: '{{ env:LOCAL_ENV_VALUE }}'
- name: EXAMPLE3                            # (3) env variable from a key in Secret
  value: '{{ secret:mysecret:key }}'
- name: EXAMPLE4                            # (3) env variable from a key in ConfigMap
  value: '{{ configMap:myconfigmap:key }}'
- value: '{{ secret:mysecret2 }}'           # (4) all key-value pairs in Secret as env variables
- value: '{{ configMap:myconfigmap2 }}'     # (4) all key-value pairs in ConfigMap as env variables
```

### `image`

This is the image name for your function after it has been built. This field
may be modified and `func` will create your image with the new name the next
time you run `kn func build` or `kn func deploy`.

### `imageDigest`

This is the `sha256` hash of the image manifest when it is deployed. This value
should not be modified.

### `labels`

The `labels` field allows you to set labels on a deployed function. Labels can be set
directly from a value or from a local environment value. Eg. `'{{ env:USER }}'`, for more details see [Local Environment Variables section](#local-environment-variables).

```yaml
labels:
- key: role                                # (1) label directly from a value
  value: backend
- key: author                              # (2) label from a local environment value
  value: '{{ env:USER }}'
```

### `name`

The name of your function. This value will be used as the name for your service
when it is deployed. This value may be changed to rename the function on
subsequent deployments.

### `namespace`

The Kubernetes namespace where your function will be deployed.


### `serviceAccountName`

The name of the service account used for the function pod. The service account
must exist in the namespace to succeed.

More info: https://k8s.io/docs/tasks/configure-pod-container/configure-service-account

### `options`
Options allows you to set specific configuration for the deployed function, allowing you to tweak Knative Service options related to autoscaling and other properties. If these options are not set, the Knative defaults will be used.
- `scale`
  - `min`: Minimum number of replicas. Must me non-negative integer, default is 0. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/scale-bounds/#lower-bound).
  - `max`: Maximum number of replicas. Must me non-negative integer, default is 0 - meaning no limit. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/scale-bounds/#upper-bound).
  - `metric`: Defines which metric type is watched by the Autoscaler. Could be `concurrency` (default) or `rps`. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/autoscaling-metrics/).
  - `target`: Recommendation for when to scale up based on the concurrent number of incoming request. Defaults to `options.resources.limits.concurrency` when given. Can be float value greater than 0.01, default is 100. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/concurrency/#soft-limit).
  - `utilization`: Percentage of concurrent requests utilization before scaling up. Can be float value between 1 and 100, default is 70. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/concurrency/#target-utilization).
  - `kpa`: Knative-specific autoscaling config, used only with `deployer: knative`. Alternative location for `metric`, `target` and `utilization` above, kept separate so KPA-specific settings don't get confused with KEDA's.
    - `metric`, `target`, `utilization`: same meaning as above.
  - `keda`: KEDA-specific scaling config, required when `deployer: keda`.
    - `triggers`: a list of KEDA triggers. At least one is required. Each trigger has a `type` of `http`, `kafka`, or `cron`:
      - `http`: scales based on incoming HTTP request rate. No additional fields.
      - `kafka`: scales based on consumer group lag. Requires [`run.kafka`](#runkafka) to be configured.
        - `lagThreshold`: average consumer lag per partition that triggers scaling up. Default is 10.
        - `activationLagThreshold`: lag below which KEDA keeps replicas at 0 when `scale.min` is 0. Default is 0.
      - `cron`: scales based on a time window.
        - `timezone`: e.g. `Europe/Istanbul`.
        - `start`, `end`: cron expressions defining the active window, e.g. `0 8 * * *`.
        - `desiredReplicas`: number of replicas to scale to during the active window.
- `resources`
  - `requests`
    - `cpu`: A CPU resource request for the container with deployed function. See related [Kubernetes docs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits).
    - `memory`: A memory resource request for the container with deployed function. See related [Kubernetes docs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits).
  - `limits`
    - `cpu`: A CPU resource limit for the container with deployed function. See related [Kubernetes docs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits).
    - `memory`: A memory resource limit for the container with deployed function. See related [Kubernetes docs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits).
    - `concurrency`: Hard Limit of concurrent requests to be processed by a single replica. Can be integer value greater than or equal to 0, default is 0 - meaning no limit. See related [Knative docs](https://knative.dev/docs/serving/autoscaling/concurrency/#hard-limit).

```yaml
options:
  scale:
    min: 0
    max: 10
    metric: concurrency
    target: 75
    utilization: 75
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 1000m
      memory: 256Mi
      concurrency: 100
```

Example using `deployer: keda` with an HTTP trigger and a Kafka trigger:

```yaml
deployer: keda
options:
  scale:
    min: 0
    max: 10
    keda:
      triggers:
        - type: http
        - type: kafka
          lagThreshold: 5
          activationLagThreshold: 0
```

Example using `deployer: knative` with explicit KPA settings:

```yaml
deployer: knative
options:
  scale:
    min: 1
    max: 10
    kpa:
      metric: concurrency
      target: 50
```

### `run.kafka`

When set, the function is deployed as a Kafka consumer: it reads CloudEvents from a Kafka
topic instead of (or, with the KEDA `http` trigger, in addition to) serving HTTP requests.
Requires `invoke: cloudevent` and the Go runtime.

- `brokers`: comma-separated list of Kafka broker addresses.
- `topic`: the topic to consume.
- `consumerGroup`: the Kafka consumer group ID.
- `securityProtocol`: one of `PLAINTEXT`, `SSL`, `SASL_PLAINTEXT`, `SASL_SSL`.
- `tls`: TLS configuration, required for `SSL` and `SASL_SSL`.
  - `caCert`: path to the CA certificate PEM file used to verify the broker certificate. Typically mounted via [`volumes`](#volumes).
  - `clientCert`, `clientKey`: paths to the client certificate/key PEM files, for mutual TLS.
  - `skipVerify`: skip broker certificate verification (development only).
- `sasl`: SASL configuration, required for `SASL_PLAINTEXT` and `SASL_SSL`.
  - `mechanism`: one of `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`.
  - `user`: SASL username. Supports `{{ secret:name:key }}` and `{{ configMap:name:key }}` syntax, or a plain value.
  - `password`: SASL password. Supports `{{ secret:name:key }}` and `{{ configMap:name:key }}` syntax.

```yaml
run:
  kafka:
    brokers: "my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9093"
    topic: "my-topic"
    consumerGroup: "my-function-group"
    securityProtocol: "SASL_SSL"
    tls:
      caCert: "/etc/kafka/ca/ca.crt"
    sasl:
      mechanism: "SCRAM-SHA-512"
      user: "my-kafka-user"
      password: "{{ secret:my-kafka-user:password }}"
  volumes:
    - secret: my-cluster-cluster-ca-cert
      path: /etc/kafka/ca
```

### `runtime`

The language runtime for your function. For example `python`.

### `template`

The source code template tailored for the invocation event that triggers
your function. For example `http` for plain HTTP requests, `event` for
CloudEvent triggered functions.

### `volumes`
Kubernetes Secrets or ConfigMaps can be mounted to the function as a Kubernetes Volume accessible under specified path. Below you can see an example how to mount the Secret `mysecret` to the path `/workspace/secret` and the ConfigMap `myconfigmap` to the path `/workspace/configmap`. This Secret/ConfigMap needs to be created before it is referenced in a function.

```yaml
volumes:
- secret: mysecret
  path: /workspace/secret
- configMap: myconfigmap
  path: /workspace/configmap
```


## Local Environment Variables

Any of the fields in `func.yaml` may contain a reference to an environment
variable available in the local environment. For example, if I would like
to avoid storing sensitive information such as an API key in my function
configuration, I may have this value set from the local environment. To do
this, prefix the local environment variable with `{{` and `}}` and prefix
the name with `env:`. For example:

```yaml
envs:
- name: API_KEY
  value: '{{ env:API_KEY }}'
```
