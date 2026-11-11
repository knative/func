# Contributor guide

This page is for people working on Kafka support itself — the `func` code that
injects and configures the runtime, and the [func-adapter-kafka][adapter] runtime
it runs. If you are writing a function that *consumes* Kafka, see
[getting-started.md](getting-started.md) and [local-development.md](local-development.md)
instead.

For how the pieces fit together, read [architecture.md](architecture.md) first;
this page covers the day-to-day workflow of changing and testing them.

## Where the code lives

- **Schema & validation** — the `run.kafka` struct and its deploy-time checks:
  `pkg/functions/function.go`.
- **Injection & wiring** — the sidecar container, env, and volume wiring, plus the
  default image pin: `pkg/k8s/deployer.go` (the keda deployer embeds this, so both
  `raw` and `keda` inject the sidecar; `knative` rejects `run.kafka`).
- **The runtime** — consuming, record→CloudEvent conversion, delivery, and offset
  commits live in the separate [func-adapter-kafka][adapter] repo, not here.

## The func ↔ adapter contract

`func` and the adapter are coupled only by a small contract; keep both sides in
sync when you change it:

- **Connection env** — `KAFKA_*` variables carry brokers, topic, group, and
  security settings.
- **Delivery env** — `FUNCTION_DELIVERY_*` and `FUNCTION_REQUEST_TIMEOUT` tune
  retry/backoff/timeout. `FUNCTION_TARGET` is where the adapter POSTs
  (`http://127.0.0.1:8080/` by default).
- **Ports** — the function serves on `8080`; the adapter serves its own health
  endpoint on `8081`.

All of these are set on the sidecar container, not the function container. See
[architecture.md](architecture.md) for the full mapping.

## Overriding the runtime image

By default the sidecar image is pinned to a released adapter version
(`DefaultKafkaRuntimeImage` in `pkg/k8s/deployer.go`). To run a different image —
a local build, a pre-release tag, or an air-gapped mirror — set
`FUNC_KAFKA_RUNTIME_IMAGE` at deploy time:

```bash
FUNC_KAFKA_RUNTIME_IMAGE=registry.example.com/kafka-runtime:dev func deploy
```

This is the main hook for testing an adapter change end to end against `func`:
build and push an image from the [func-adapter-kafka][adapter] repo (it is
`ko`-built; see that repo for the current build commands), then point `func` at it
with `FUNC_KAFKA_RUNTIME_IMAGE` and run the end-to-end walkthrough below.

## Image pin and release lockstep

`func` pins the **released** adapter tag, not a nightly, so a given `func` build
always runs a known runtime. When a new adapter version is released, bump
`DefaultKafkaRuntimeImage` in a follow-up `func` change; the override above is for
testing before that bump lands, not a substitute for it.

## Testing end to end

This walkthrough runs a Kafka function end to end on a local
[kind](https://kind.sigs.k8s.io/) cluster with [Strimzi](https://strimzi.io/)
providing Kafka. It verifies the whole path: produce a record → the sidecar
consumes it → the function receives it as a CloudEvent → the offset is committed.

To keep the test dependency-light it uses the **`raw`** deployer with a fixed
replica, which exercises consumption and delivery without needing the KEDA operator
installed. For consumer-lag autoscaling behavior, see
[deployment-and-scaling.md](deployment-and-scaling.md).

To test a **locally-built adapter** instead of the pinned release, set
`FUNC_KAFKA_RUNTIME_IMAGE` (see above) before `func deploy` in step 4.

### Prerequisites

- `kind`, `kubectl`, and the `func` CLI installed.
- A Docker daemon.
- A container registry the cluster can pull your **function** image from (the
  Kafka runtime sidecar uses its released default image, so no extra image is
  needed). Set it once:

```bash
export FUNC_REGISTRY=docker.io/<you>   # a registry you can push to and pull from
```

### 1. Create a cluster

```bash
kind create cluster --name kafka-test
```

### 2. Install Strimzi and a Kafka cluster

```bash
kubectl create namespace kafka
kubectl apply -f 'https://strimzi.io/install/latest?namespace=kafka' -n kafka
kubectl wait --for=condition=Ready pods --all -n kafka --timeout=180s

kubectl apply -n kafka -f - <<'EOF'
apiVersion: kafka.strimzi.io/v1
kind: KafkaNodePool
metadata:
  name: dual-role
  labels:
    strimzi.io/cluster: my-cluster
spec:
  replicas: 1
  roles: [controller, broker]
  storage:
    type: jbod
    volumes:
      - id: 0
        type: persistent-claim
        size: 1Gi
        deleteClaim: true
---
apiVersion: kafka.strimzi.io/v1
kind: Kafka
metadata:
  name: my-cluster
  annotations:
    strimzi.io/node-pools: enabled
    strimzi.io/kraft: enabled
spec:
  kafka:
    version: 4.2.0
    listeners:
      - name: plain
        port: 9092
        type: internal
        tls: false
    config:
      offsets.topic.replication.factor: 1
      transaction.state.log.replication.factor: 1
      transaction.state.log.min.isr: 1
  entityOperator:
    topicOperator: {}
    userOperator: {}
EOF

kubectl wait kafka/my-cluster --for=condition=Ready --timeout=300s -n kafka
```

Create a topic:

```bash
kubectl apply -n kafka -f - <<'EOF'
apiVersion: kafka.strimzi.io/v1
kind: KafkaTopic
metadata:
  name: test-topic
  labels:
    strimzi.io/cluster: my-cluster
spec:
  partitions: 3
  replicas: 1
EOF
```

The in-cluster broker address is
`my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9092`.

### 3. Create and configure a function

```bash
func create -l <language> -t cloudevents test-func
cd test-func
```

Edit `func.yaml` to add the deployer, invoke mode, a fixed replica, and the
`run.kafka` block (PLAINTEXT, so no security settings):

```yaml
deployer: raw
invoke: cloudevent
scale:
  min: 1
run:
  kafka:
    brokers: "my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9092"
    topic: "test-topic"
    consumerGroup: "test-func"
```

The default CloudEvents handler already logs each event, so no code changes are
needed for the test.

### 4. Deploy

```bash
func deploy
```

Confirm both containers are present and the pod rolled out:

```bash
kubectl get deployment test-func \
  -o jsonpath='{.spec.template.spec.containers[*].name}{"\n"}'
# expected: user-container kafka-runtime

kubectl rollout status deployment/test-func --timeout=120s
```

### 5. Produce a record and verify

The runtime consumes from the **newest** offset, so wait for the consumer to be
ready **before** producing — otherwise your record is sent before the group joins
and is skipped:

```bash
kubectl logs deployment/test-func -c kafka-runtime | grep -i "consumer ready"
```

Produce a record (pipe the input on stdin — an interactive producer often exits
before flushing):

```bash
echo "hello from kafka" | kubectl -n kafka run kprod --rm -i --restart=Never \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-console-producer.sh \
    --bootstrap-server my-cluster-kafka-bootstrap:9092 \
    --topic test-topic
```

Verify the function received it and lag drained to zero:

```bash
# The function received the CloudEvent
kubectl logs deployment/test-func -c user-container --tail=20

# Consumer-group lag should be 0 across all partitions
kubectl run kafka-lag -n kafka --rm -i --restart=Never \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-consumer-groups.sh \
    --bootstrap-server my-cluster-kafka-bootstrap:9092 \
    --describe --group test-func
```

Seeing the event in the function log and `LAG 0` confirms the full path works.

### 6. Clean up

```bash
func delete
kind delete cluster --name kafka-test
```

### Testing a secured broker

The steps above use a PLAINTEXT listener for simplicity. To test `SASL_SSL` (or
`SSL`), configure a Strimzi TLS listener and a `KafkaUser`, copy the CA and user
Secrets into the function's namespace, and set `securityProtocol`, `tls.caCert`,
and `sasl.*` in `run.kafka` with the CA mounted via `run.volumes` — see
[getting-started.md](getting-started.md#3-provide-credentials-secured-brokers).

If something doesn't work, see [troubleshooting.md](troubleshooting.md).

## Local sub-process mode (in progress)

A local mode that runs the adapter as a sub-process beside `func run` — so a
locally-run function can consume from a real broker without a cluster — is being
prototyped. It is tracked in
[knative/func#4124](https://github.com/knative/func/issues/4124).

[adapter]: https://github.com/knative-extensions/func-adapter-kafka
