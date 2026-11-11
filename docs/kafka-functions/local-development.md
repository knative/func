# Local development

This page covers how to develop and test a Kafka function on your machine, and what
is and isn't available locally today.

## What runs locally

`func run` starts your function locally as a plain CloudEvents-over-HTTP server. The
**Kafka runtime sidecar is not part of a local run**, so the managed sidecar
delivery path does not run locally — for most languages this means a locally-run
function does **not** connect to a broker or consume any topic, and `run.kafka`'s
sidecar wiring only takes effect when the function is deployed to a cluster.

> **Go is an exception.** Go functions still carry the legacy in-process Kafka
> consumer. When `run.kafka` is fully configured, `func run` sets
> `FUNC_TRANSPORT=kafka` (plus `KAFKA_*`) and the Go scaffold connects **directly** to
> the broker and consumes the topic in-process — so a Go local run *does* talk to
> Kafka if the broker is reachable (and if it is **not** reachable, `func run` never
> becomes ready and eventually fails with a timeout). This path is being retired in
> favor of the sidecar.
>
> One consequence: while `run.kafka` is set, the in-process Kafka server registers
> only `/health/readiness` and `/health/liveness` — **not** `/`. So the `func invoke`
> approach below does **not** reach a Go handler (the POST to `/` returns 404, even
> though `func invoke` may still report success). To exercise a Go handler with
> `func invoke`, temporarily remove `run.kafka` from `func.yaml` so `func run` starts
> a plain CloudEvents HTTP server — or just test the handler with an ordinary Go test.
> For every **other** language `func run` always starts the plain HTTP server, so
> `func invoke` works as described.

That is fine for most development, because your function's job is simply to handle a
CloudEvent. You can exercise that logic locally by sending it a CloudEvent shaped
like one the runtime would deliver.

## Exercising the handler with `func invoke`

Run the function in one terminal:

```bash
func run
```

In another terminal, invoke it with a CloudEvent that mimics a Kafka-delivered
record — the `type`, `source`, content type, and Kafka extensions the runtime would
set (see [architecture.md](architecture.md#record--cloudevent-mapping)):

```bash
func invoke \
  --format cloudevent \
  --type dev.knative.kafka.event \
  --source "kafka://my-cluster/orders" \
  --content-type application/octet-stream \
  --data '{"orderId":"123","total":42}' \
  --extension kafkatopic=orders \
  --extension kafkapartition=0 \
  --extension kafkaoffset=17
```

This drives your handler through the shape it receives in production, without needing
a broker. Use `--file` instead of `--data` to send a payload from a file. (The
`--extension`/`-e` flag is only valid with the cloudevent format.)

**Content type matters.** A **plain** Kafka record is *always* delivered with its data
as `application/octet-stream` (raw bytes) — the runtime never sets `application/json`
for a plain record, even when the value is JSON and even when the producer set a
`ce_datacontenttype` header. So invoke with `application/octet-stream` and have your
handler read the raw bytes (parse them as JSON yourself if needed). Testing with
`application/json` would exercise a shape the runtime never produces: a handler that
decodes by content type could pass locally and then fail in the cluster.

A record is delivered as its *own* CloudEvent only when the producer wrote CloudEvent
headers (the runtime keys off `ce_specversion`) **and** those headers form a valid,
header-safe CloudEvent. In that case it keeps the **producer's** `id`, `type`,
`source`, and content type, and gets **no** `kafka*` extensions. If the `ce_*` values
are malformed or header-unsafe (for example an empty `ce_id`, an unknown
`ce_specversion`, or a value with control characters), the runtime falls back to the
plain-wrapping case above instead. Mimic the valid pass-through case separately:

```bash
func invoke \
  --format cloudevent \
  --type com.example.order.created \
  --source "/orders/service" \
  --content-type application/json \
  --data '{"orderId":"123","total":42}'
```

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
