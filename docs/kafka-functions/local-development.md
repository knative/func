# Local development

This page covers how to develop and test a Kafka function on your machine, and what
is and isn't available locally today.

## What runs locally

`func run` starts your function locally as a plain CloudEvents-over-HTTP server. The
**Kafka runtime sidecar is not part of a local run**, so a locally-run function
does **not** connect to a broker or consume any topic — `run.kafka` only takes
effect when the function is deployed to a cluster.

That is fine for most development, because your function's job is simply to handle a
CloudEvent. You can exercise that logic locally by sending it a CloudEvent shaped
like one the runtime would deliver.

## Exercising the handler with `func invoke`

Run the function in one terminal:

```bash
func run
```

In another terminal, invoke it with a CloudEvent that mimics a Kafka-delivered
record — same `type`, `source`, and Kafka extensions the runtime would set (see
[architecture.md](architecture.md#record--cloudevent-mapping)):

```bash
func invoke \
  --format cloudevent \
  --type dev.knative.kafka.event \
  --source "kafka://my-cluster/orders" \
  --content-type application/json \
  --data '{"orderId":"123","total":42}' \
  --extension kafkatopic=orders \
  --extension kafkapartition=0 \
  --extension kafkaoffset=17
```

This drives your handler through exactly the shape it will receive in production,
without needing a broker. Use `--file` instead of `--data` to send a payload from a
file. (The `--extension`/`-e` flag is only valid with the cloudevent format.)

## Testing against a real broker

To exercise the full path — real consumption, offset commits, retries, scaling —
deploy to a cluster. See the
[contributor guide](contributor-guide.md#testing-end-to-end) for a complete local
kind + Strimzi walkthrough.

## Planned: local sub-process mode

A local mode that runs the actual Kafka runtime as a sub-process alongside
`func run` — so a locally-run function can consume from a real broker without a
cluster — is planned and tracked in
[knative/func#4124](https://github.com/knative/func/issues/4124). Until it ships,
use `func invoke` for handler logic and a cluster deploy for real Kafka.
