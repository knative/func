# Kafka functions

Documentation for writing, configuring, and deploying functions that consume
Apache Kafka.

A Kafka function is an **ordinary function in any supported language** — it does
not embed a Kafka client and uses no Kafka-specific template. You declare a
`run.kafka` block in `func.yaml`, and at deploy time the platform injects a
language-agnostic **runtime sidecar** next to your function container. The sidecar
consumes the topic, converts each record to a CloudEvent, and POSTs it to your
function over localhost; your function stays a plain CloudEvents-over-HTTP server.

Delivery is **at-least-once** — the sidecar commits a record's offset only after
your function returns a 2xx. The recommended deployer is `keda`, which adds
consumer-lag autoscaling (including scale-to-zero); the `raw` deployer also injects
the sidecar but runs a fixed replica count with no lag scaling. The `knative`
deployer does not support Kafka functions.

New to this? Start with [Getting started](getting-started.md).

## Pages

| Page                                                              | What it covers                                                               |
|-------------------------------------------------------------------|------------------------------------------------------------------------------|
| [Getting started](getting-started.md)                             | Create, configure, deploy, and verify a Kafka function end to end.           |
| [Delivery, retries & offsets](delivery-and-retries.md)            | At-least-once delivery, the retry budget, backoff, timeouts, and exhaustion. |
| [Deployment & scaling](deployment-and-scaling.md)                 | Deployer choice, what gets deployed, and consumer-lag autoscaling.           |
| [Idempotency & deduplication](idempotency.md)                     | Handling the duplicate deliveries that at-least-once implies.                |
| [Local development](local-development.md)                         | Running and invoking a Kafka function during development.                    |
| [Using a Kafka client directly](using-a-kafka-client-directly.md) | When to opt out of the runtime and talk to Kafka yourself.                   |
| [Troubleshooting](troubleshooting.md)                             | Diagnosing a consumer that isn't receiving or scaling.                       |
| [Architecture](architecture.md)                                   | How Kafka support is built (for contributors).                               |
| [Contributor guide](contributor-guide.md)                         | Working on func's Kafka support and the adapter runtime.                     |

The full `run.kafka` field reference lives in the
[`func.yaml` reference](../reference/func_yaml.md#runkafka).

## Not yet covered

These are planned or possible future capabilities; this documentation will cover
them once they ship:

- Dead-letter routing for records that exhaust their retry budget.
- Producing or replying to Kafka from a function.
- Batch delivery, pluggable deserialization, and CloudEvent format detection.
- In-function concurrency control.
- Metrics and observability for the runtime.
