# Getting started with Kafka functions

This walkthrough takes you from zero to a deployed function that consumes records
from a Kafka topic. Kafka support is **language-agnostic**: your function is an
ordinary CloudEvents handler, and a runtime sidecar injected at deploy time does
the Kafka consuming for you. You do not write any Kafka client code, and there is
no Kafka-specific template.

For the full `run.kafka` field reference see
[`func.yaml` reference](../reference/func_yaml.md#runkafka); for how records are
delivered and retried see [delivery-and-retries.md](delivery-and-retries.md).

## Prerequisites

- The `func` CLI installed.
- A cluster. This guide uses the **`keda`** deployer, which is recommended for
  Kafka functions because it adds consumer-lag autoscaling and scale-to-zero.
  `run.kafka` also works with the `raw` deployer (a fixed replica count, no
  lag-based scaling); it is **not** supported on the `knative` deployer. See
  [deployment-and-scaling.md](deployment-and-scaling.md).
- A reachable Kafka broker and a topic to consume.
- If the broker requires authentication, the TLS CA certificate and/or SASL
  credentials for it (added in step 3).

## 1. Create a function

Create a normal CloudEvents function in the language of your choice. There is **no
Kafka template** — a Kafka function is just a CloudEvents handler that the sidecar
delivers records to.

```bash
func create -l <language> -t cloudevents myfunc
cd myfunc
```

Replace `<language>` with any supported language that has a `cloudevents`
template (for example `go`, `node`, `python`, `quarkus`, `rust`, `springboot`,
`typescript`). Write your handler as you normally would for CloudEvents; each
consumed Kafka record arrives as one CloudEvent.

## 2. Declare Kafka in `func.yaml`

Add a `run.kafka` block, select the `keda` deployer, and add a `kafka` scale
trigger so the function autoscales on consumer-group lag. The `run.kafka` block
needs three fields — the brokers, the topic, and the consumer group:

```yaml
deployer: keda
scale:
  min: 0
  max: 3
  keda:
    triggers:
      - type: kafka        # scale on consumer-group lag
        lagThreshold: 5
run:
  kafka:
    brokers: my-cluster-kafka-bootstrap:9092
    topic: orders
    consumerGroup: myfunc
```

- `brokers` — comma-separated broker addresses.
- `topic` — the topic to consume from.
- `consumerGroup` — the Kafka consumer group ID. Give each function its own group.

The `kafka` trigger is what makes the Deployment scale on consumer lag (including
scaling to zero when `scale.min` is `0`). **Omit the whole `scale.keda` block and the
`keda` deployer falls back to a default HTTP scaler** bounded by `scale.min` /
`scale.max`: the function still consumes Kafka, but it scales on HTTP traffic, not
lag. With `scale.min: 0` — as in this example — that means it scales to **zero when
there is no HTTP traffic, and stops consuming**; set `scale.min: 1` to keep it
consuming without a kafka trigger. (Removing only the trigger item is not an option —
`scale.keda` with an empty `triggers` list is rejected at validation.) See
[deployment-and-scaling.md](deployment-and-scaling.md#consumer-lag-autoscaling) for
`lagThreshold`, `activationLagThreshold`, and the partition cap on `scale.max`.

You can also select the deployer per-invocation with `func deploy --deployer keda`
(or `FUNC_DEPLOYER=keda`) instead of setting it in `func.yaml`.

## 3. Provide credentials (secured brokers)

Skip this step if your broker accepts plaintext connections. For a secured broker,
set `securityProtocol` and supply credentials.

Mount the broker's CA certificate from a Secret and point `tls.caCert` at the
mounted path. Supply SASL credentials with the `{{ secret:name:key }}` reference
syntax so they are read from a Secret rather than written in `func.yaml`:

```yaml
deployer: keda
run:
  volumes:
    - secret: kafka-ca          # Secret containing ca.crt
      path: /etc/kafka/ca
  kafka:
    brokers: my-cluster-kafka-bootstrap:9093
    topic: orders
    consumerGroup: myfunc
    securityProtocol: SASL_SSL
    tls:
      caCert: /etc/kafka/ca/ca.crt
    sasl:
      mechanism: SCRAM-SHA-512
      user: "{{ secret:kafka-credentials:user }}"
      password: "{{ secret:kafka-credentials:password }}"
```

Create the referenced Secrets in the function's namespace beforehand (pass the same
`-n <namespace>` you deploy into, since these commands otherwise target your current
`kubectl` namespace), for example:

```bash
kubectl create secret generic kafka-ca -n my-namespace --from-file=ca.crt=./ca.crt
kubectl create secret generic kafka-credentials -n my-namespace \
  --from-literal=user=myuser --from-literal=password=mypassword
```

`securityProtocol` accepts `PLAINTEXT`, `SSL`, `SASL_PLAINTEXT`, or `SASL_SSL`.
See the [`func.yaml` reference](../reference/func_yaml.md#runkafka) for every TLS
and SASL option.

## 4. Deploy

```bash
func deploy
```

Deploy builds and pushes the image, then creates a KEDA-scaled Deployment with the
Kafka runtime sidecar injected alongside your function container. The deploy
reports success once the function is up and the sidecar has connected to the
broker and been assigned partitions — so a green deploy confirms the Kafka wiring,
not just that the image started.

## 5. Send a record and verify

Produce a record to the topic (any Kafka producer works). With the Strimzi
console producer, for example:

```bash
kubectl run kafka-producer -it --rm --restart=Never \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-console-producer.sh \
  --bootstrap-server my-cluster-kafka-bootstrap:9092 --topic orders
```

Then watch the function receive it:

```bash
func logs
```

You should see your handler log the incoming CloudEvent. Consumer-group lag for
the topic returns to zero once the record is delivered and its offset committed.
For an end-to-end local setup (kind + Strimzi), see the
[contributor guide](contributor-guide.md#testing-end-to-end).

## What just happened

The `keda` deployer injected a runtime sidecar next to your function container.
The sidecar consumes records from the topic, converts each one to a CloudEvent,
and POSTs it to your function over localhost. It commits the Kafka offset only
after your function returns a 2xx response — so delivery is **at-least-once**: a
record is retried until it is accepted, which means your function can occasionally
see the same record more than once. See
[architecture.md](architecture.md) for the full model and
[idempotency.md](idempotency.md) for how to handle duplicates.

## Next steps

- [`func.yaml` reference](../reference/func_yaml.md#runkafka) — every `run.kafka` field.
- [delivery-and-retries.md](delivery-and-retries.md) — retry budget, backoff, timeouts.
- [deployment-and-scaling.md](deployment-and-scaling.md) — the KEDA requirement and
  consumer-lag autoscaling.
- [idempotency.md](idempotency.md) — handling at-least-once duplicates.
