# Local development

This page covers how to develop and test a Kafka function on your machine.

## What runs locally

When `run.kafka` is fully configured (brokers, topic and consumer group), `func run`
starts the **Kafka runtime** as a process next to your function. It is the same
program, at the same version, that runs as the sidecar when the function is deployed:
it consumes the topic and POSTs each record to the function as a CloudEvent,
committing the offset only on a `2xx`, with the same `run.kafka.delivery` retry,
backoff and timeout settings. Your function runs as a plain CloudEvents-over-HTTP
server, exactly as it does on the cluster. This works the same for every language,
whether the function runs as a host process (`--builder host`) or in a container.

`func run` reports the function running only once the runtime has joined the consumer
group and been assigned partitions. If the broker is unreachable, the topic does not
exist or the credentials are rejected, `func run` fails with the runtime's error
rather than starting a function that silently receives nothing. The runtime's output
is shown in the `func run` output prefixed with `[kafka]`, and it stops with the
function.

```console
$ func run
...
[kafka] {"level":"info","brokers":["localhost:9092"],"topic":"orders","group":"orders-dev","message":"connecting to kafka"}
[kafka] {"level":"info","message":"kafka consumer ready (partitions assigned)"}
Function running on 127.0.0.1:8080
```

### Configuring a local run

`func run` uses the `run.kafka` block from `func.yaml` as it is, so check it makes
sense from your machine:

- **Brokers** must be reachable from your machine. An in-cluster address such as
  `my-cluster-kafka-bootstrap.kafka:9092` is not; point at a broker you can reach,
  for example a single-node broker in a local container.
- **Consumer group.** The local runtime joins `run.kafka.consumerGroup`. If the
  function is also deployed against the same broker, the local run joins the same
  group and takes over some of its partitions while it runs. A group with no
  committed offsets starts at the end of the topic, so only records produced after
  `func run` reports the function running are delivered to a new group.
- **TLS files** (`tls.caCert`, `tls.clientCert`, `tls.clientKey`) are read by the
  runtime from your machine's filesystem, so for a local run they must be paths on
  your machine. (Deployed, they are paths inside the sidecar, mounted from
  `run.volumes`.)
- **SASL credentials** given as inline values are used as is. A
  `{{ secret:name:key }}` or `{{ configMap:name:key }}` reference names a cluster
  resource that `func run` cannot read: it is passed to the runtime verbatim, so
  authentication fails, and `func run` warns about it beforehand. A local run
  against a SASL-secured broker therefore needs inline credentials.

### Where the runtime comes from

`func` release builds for macOS, Linux and Windows on amd64 and arm64 include the
runtime and extract it on first use into `kafka-runtime/<version>/` under func's
config directory (`~/.config/func`, or `$XDG_CONFIG_HOME/func`). On other platforms,
or with a `func` built without it (for example with a plain `go build`), `func run`
warns that records will not be delivered and runs the function alone. To use a
runtime binary of your own instead — on such a platform, or to test a locally built
runtime — set `FUNC_KAFKA_RUNTIME_BINARY` to the path of an `fkafka-runtime` binary
built from [func-adapter-kafka](https://github.com/knative-extensions/func-adapter-kafka).

## Exercising the handler with `func invoke`

You can also drive your handler directly, without producing to Kafka, by sending it
a CloudEvent shaped like one the runtime delivers. (Without a reachable broker,
remove `run.kafka` from `func.yaml` while you do this, since `func run` fails when
it cannot start the runtime.)

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

## Testing on a cluster

Consumer-lag autoscaling, and the deployed setup as a whole, can only be exercised
on a cluster. See the [contributor guide](contributor-guide.md#testing-end-to-end)
for a complete local kind + Strimzi walkthrough.
