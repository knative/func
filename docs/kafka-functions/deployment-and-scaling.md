# Deployment & scaling

This page covers how a Kafka function is deployed, what ends up running on the
cluster, and how it scales on consumer-group lag. For the exact `scale.keda`
field schema, see the [`func.yaml` reference](../reference/func_yaml.md#scale);
this page is about behavior and trade-offs.

## Which deployer to use

`run.kafka` is supported on two deployers:

- **`keda` (recommended)** — deploys a Kubernetes Deployment scaled by
  [KEDA](https://keda.sh). This is the only deployer that gives you
  consumer-lag autoscaling and scale-to-zero, so it is the right choice for most
  Kafka functions.
- **`raw`** — deploys a plain Kubernetes Deployment with a fixed replica count.
  The runtime sidecar is still injected and the function consumes normally; there
  is just no lag-based autoscaling. Useful when you want a constant number of
  consumers.

`run.kafka` is **not** supported on the `knative` deployer: a Knative Service
scales on HTTP traffic, which has nothing to do with Kafka lag, so deploying there
would produce a function that never reliably receives records. The knative
deployer rejects `run.kafka` at deploy time and tells you to switch to `keda`.

Select the deployer in `func.yaml` (`deployer: keda`) or per command
(`func deploy --deployer keda`, or `FUNC_DEPLOYER=keda`).

## What gets deployed

On the `keda` deployer, a single deploy creates:

- A **Deployment** whose pod runs **two containers**: your function container and
  the injected `kafka-runtime` sidecar. The sidecar owns the Kafka connection and
  delivers records to the function over localhost; your container stays a plain
  CloudEvents-over-HTTP server. See [architecture.md](architecture.md).
- A KEDA **`ScaledObject`** (for a `kafka` trigger) or **`HTTPScaledObject`** (for
  an `http` trigger) that drives autoscaling.
- A **`TriggerAuthentication`** when a `kafka` trigger needs credentials to read
  consumer-group lag from a secured broker.

The sidecar has a startup probe that gives it time to connect to the broker and be
assigned partitions before health checks begin, so a successful deploy means the
Kafka wiring is up — not merely that the container started.

## Consumer-lag autoscaling

A `kafka` trigger scales the Deployment on **consumer-group lag** — how far behind
the topic the group is. Add it under `scale.keda.triggers`:

```yaml
deployer: keda
scale:
  min: 0
  max: 3
  keda:
    triggers:
      - type: kafka
        lagThreshold: 5
        activationLagThreshold: 0
run:
  kafka:
    brokers: my-cluster-kafka-bootstrap:9093
    topic: orders
    consumerGroup: myfunc
```

- **`lagThreshold`** — the average lag per partition KEDA aims to hold. Lower it to
  scale up more eagerly, raise it to tolerate more backlog before adding replicas.
- **`activationLagThreshold`** — the lag below which KEDA keeps the Deployment at
  zero (only relevant when `scale.min` is 0).

A `kafka` trigger requires `run.kafka` to be configured; `func` rejects the
combination otherwise.

### Partitions cap useful parallelism

Within a consumer group, each partition is consumed by at most one member, so a
topic with *N* partitions supports at most *N* actively-consuming replicas. Setting
`scale.max` higher than the partition count just creates idle replicas that never
get a partition assigned. **Keep `scale.max` ≤ the topic's partition count.**

## Scale-to-zero and minimum replicas

With a **`kafka` trigger** and `scale.min: 0`, the Deployment scales to zero when
lag is at or below `activationLagThreshold`, and scales back up when records
arrive. This is the typical event-driven setup: no records, no running pods.

Be careful mixing scale-to-zero with a **non-Kafka** trigger. With an `http`
trigger (or the implicit HTTP default) and `scale.min: 0`, KEDA scales to zero
whenever there is no *HTTP* traffic. That stops the consumer, and since nothing
sends it HTTP requests, lag grows with nothing to wake it back up. For a Kafka
consumer, either use a `kafka` trigger (so lag itself drives scaling) or set
`scale.min: 1` to keep at least one consumer running.

Note that `http` and `kafka` triggers cannot be combined in the same function —
KEDA allows only one scaler per workload.

## Credentials

TLS and SASL credentials are supplied through `run.kafka` and must exist as
Secrets in the function's namespace before you deploy. The CA certificate is
mounted from a Secret via `run.volumes`; SASL credentials use the
`{{ secret:name:key }}` reference syntax. See
[getting-started.md](getting-started.md#3-provide-credentials-secured-brokers) for
a worked example and the [`func.yaml` reference](../reference/func_yaml.md#runkafka)
for every field.

When a `kafka` trigger is used against a secured broker, the same credentials are
also wired into a KEDA `TriggerAuthentication` so the scaler can read lag.

## Next steps

- [delivery-and-retries.md](delivery-and-retries.md) — how records are delivered and retried.
- [architecture.md](architecture.md) — the sidecar model and offset-commit flow.
- [troubleshooting.md](troubleshooting.md) — diagnosing a consumer that isn't scaling or receiving.
