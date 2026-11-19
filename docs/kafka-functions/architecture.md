# Architecture

This page explains how Kafka support is built, for contributors and maintainers.
User-facing usage is covered by the other pages in this folder; this one is about
the internals and the split of responsibilities between `func` and the runtime.
For the day-to-day workflow of changing and testing them, see the
[contributor guide](contributor-guide.md).

## The sidecar model

A Kafka function is an ordinary CloudEvents-over-HTTP function in any language. It
embeds no Kafka client. When `run.kafka` is set, the deployer injects a second
container into the pod — the **Kafka runtime sidecar** (built from the
[func-adapter-kafka](https://github.com/knative-extensions/func-adapter-kafka)
repo) — next to the function container:

```
┌────────────────────────── Pod ────────────────────────────┐
│  kafka-runtime (sidecar)            user-container        │
│  ┌───────────────────┐              ┌──────────────────┐  │
│  │ consume topic     │  CloudEvent  │ your function    │  │
│  │ record→CloudEvent │ ──POST────▶  │ (CE-over-HTTP)   │  │
│  │ commit on 2xx     │  :8080       │                  │  │
│  └───────────────────┘              └──────────────────┘  │
└───────────────────────────────────────────────────────────┘
        │
        ▼  Kafka broker
```

The sidecar owns everything Kafka: it consumes the topic, converts each record to
a CloudEvent, and POSTs it to the function at `http://127.0.0.1:8080/`. The
function never speaks Kafka, which is what makes the feature language-agnostic. The
sidecar serves its own health endpoint on port `8081`.

## Record → CloudEvent mapping

The runtime converts each Kafka record to a CloudEvent. As implemented by
func-adapter-kafka (which is the authoritative source for the exact mapping), a
plain record becomes:

- `type`: `dev.knative.kafka.event`
- `source`: `kafka://<brokers>/<topic>`
- `id`: `partition:<p>/offset:<o>` — stable per record, so it repeats on
  redelivery. It is built from partition and offset **only** (no topic), so it is
  not unique across topics or across a re-created topic; for deduplication across a
  shared store, compose a key from the consumer group plus the `kafkatopic` /
  `kafkapartition` / `kafkaoffset` extensions (see
  [idempotency.md](idempotency.md#choosing-a-deduplication-key))
- `time`: the record's timestamp
- `subject`: the record key as text — set **only** when the key is valid UTF-8 with
  no control characters; otherwise it is omitted (for example, a binary key)
- `data`: the raw record value, with content type `application/octet-stream`
- extensions: `kafkatopic`, `kafkapartition`, `kafkaoffset`, and — when the record
  has a key — `kafkakey`, which is the key **base64-encoded** (always, regardless of
  whether the key is valid text). Decode it to recover the raw key; a handler that
  compares `kafkakey` against the raw key bytes will never match. The readable
  `subject` is the one to compare against, and only when it is present.

If the record already carries CloudEvent headers (the runtime keys off
`ce_specversion`), it is passed through as that CloudEvent instead of being
re-wrapped — it keeps the **producer's** `id`, `type`, `source`, and content type,
and does **not** get the `dev.knative.kafka.event` type or the `kafka*` extensions
above. The pass-through only happens when those `ce_*` headers form a **valid**
CloudEvent whose values are safe to carry back out as HTTP headers; if any are
malformed or header-unsafe (for example an empty `ce_id`, an unknown
`ce_specversion`, or a value with control characters), the runtime falls back to
wrapping the record as a plain `dev.knative.kafka.event` with the fields above.

## Offset-commit strategy (at-least-once)

The sidecar commits a record's offset **only after** the function returns a 2xx.
A non-2xx, a per-attempt timeout, or a dropped connection is a failed attempt and
the offset is not advanced; the record is retried per the `delivery` settings.
This yields at-least-once delivery. When a bounded retry budget is exhausted the
record is skipped and its offset committed (with a warning), so a poison record
cannot wedge a partition forever. The user-facing behavior is described in
[delivery-and-retries.md](delivery-and-retries.md).

## How `func` injects the sidecar

Injection lives in the **raw/Kubernetes deployer** (`pkg/k8s/deployer.go`), which
the keda deployer embeds — so both `raw` and `keda` get the sidecar; the `knative`
deployer rejects `run.kafka`. Key pieces:

- **Image.** `DefaultKafkaRuntimeImage` pins the released runtime image. It can be
  overridden at deploy time with the `FUNC_KAFKA_RUNTIME_IMAGE` environment
  variable.
- **Container.** A container named `kafka-runtime` is appended to the pod. It
  targets the function at `http://127.0.0.1:8080/` and has a startup probe sized so
  a slow-starting function doesn't restart-loop the sidecar before it connects.
- **Config → env.** `AppendKafkaEnvs` maps `run.kafka` connection fields to
  `KAFKA_*` env vars, and `AppendKafkaDeliveryEnvs` maps `run.kafka.delivery` to
  `FUNCTION_DELIVERY_*` / `FUNCTION_REQUEST_TIMEOUT`. Both are built on
  `KafkaConnectionEnvs` / `KafkaDeliveryEnvs` (`pkg/functions/kafka_runtime_env.go`),
  the single mapping from `run.kafka` to the runtime's environment, which local runs
  use as well. **These land on the sidecar, not the function container** — the
  function stays plain.
- **Credentials.** TLS certificate volumes (for `tls.caCert` etc.) are mounted into
  the sidecar only, never exposed to the user container.
- **Scaling (keda).** The keda deployer additionally creates the KEDA `ScaledObject`
  and, for secured brokers, a `TriggerAuthentication` so the Kafka scaler can read
  consumer-group lag.

## Responsibilities: `func` vs the adapter

- **`func` (this repo)** owns the CLI/UX, the `run.kafka` schema and its validation
  (`pkg/functions/function.go`), sidecar injection and env/volume wiring
  (`pkg/k8s/deployer.go`), and the keda scaling resources.
- **func-adapter-kafka** owns the runtime itself: consuming records, the
  record→CloudEvent conversion, delivery with retry/backoff/timeout, and offset
  commits. Look there for the runtime internals and the exact conversion rules.

## Local runs

`func run` has no Pod to put a sidecar in, so it runs the same runtime as a
**process** next to the function (`pkg/functions/runner_kafka.go`). `Client.Run`
starts it once the runner reports the function ready, so it serves every runner
alike: the host runner's function process and the container runner's published
port.

- **Binary.** The runtime is built from the module version pinned by
  `kafkaruntime.Version` (`pkg/kafkaruntime`) and embedded into the `func` binary per
  platform (`hack/kafka-runtime.sh`, run by the Makefile build targets), then
  extracted on first use into func's config directory. The same constant builds
  `DefaultKafkaRuntimeImage`, so a local run and a deployed function use the same
  runtime version. `FUNC_KAFKA_RUNTIME_BINARY` overrides the embedded binary.
- **Config → env.** The same `KafkaConnectionEnvs` / `KafkaDeliveryEnvs` mapping as
  the sidecar, so the runtime gets the same variables locally and deployed.
  `FUNCTION_TARGET` is the function's local address and `LISTEN_ADDRESS` a free
  loopback port for the runtime's health endpoints. Inherited `KAFKA_*` /
  `FUNCTION_*` runtime settings from the user's shell are dropped, so `run.kafka`
  alone configures it, as on the cluster. SASL `{{ secret:... }}` /
  `{{ configMap:... }}` references cannot be resolved locally and are passed through
  with a warning.
- **Lifecycle.** `func run` waits for the runtime's `/health/readiness` (ready once
  partitions are assigned) within the start timeout, and fails at once if the
  runtime exits first. An unexpected exit afterwards ends the run like a function
  crash. Stopping the run stops the runtime before the function.

## The retired in-process path

Earlier, Kafka was handled in-process by language-specific middleware (a `kafka`
template, `invoke: kafka` scaffolding, and lifecycle hooks generated into the
function). That approach required per-language implementations and is retired in
favor of the runtime, which works for any language with no function-side Kafka code:
as the sidecar on the cluster, and as a process beside the function in a local run
(see [Local runs](#local-runs)). The Go scaffolding no longer contains the
in-process consumer, and `func run` no longer sets `FUNC_TRANSPORT=kafka` or any
`KAFKA_*` variable on the function. New work should target the runtime model only.

## Future direction

Planned and possible extensions — dead-letter routing, producing/replying to
Kafka, batch delivery, pluggable deserialization, CloudEvent format detection,
in-function concurrency control, and runtime metrics/observability — are tracked
in the adapter repo and its issues.
