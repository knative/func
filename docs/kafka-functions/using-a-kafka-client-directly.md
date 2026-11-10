# Using a Kafka client directly

The [runtime sidecar](architecture.md) is the recommended way to consume Kafka from
a function: you declare `run.kafka`, write no Kafka code, and get at-least-once
delivery and consumer-lag autoscaling for free. But it deliberately covers the
common consume-and-handle case. When you need something it doesn't yet offer, you
can **opt out** and talk to Kafka directly from your function code.

## When to opt out

Consider a direct Kafka client when you need:

- **Producing or replying** to Kafka from the function (the runtime currently
  consumes only).
- **Transactions / exactly-once** semantics across consume-process-produce.
- **Batch or bulk** processing of many records per call, rather than one CloudEvent
  per record.
- **Custom deserialization** — a schema registry, Avro/Protobuf, or format handling
  the runtime doesn't perform.
- **Fine-grained offset control** — manual commits, seeking, pausing/resuming
  partitions.
- **Multiple topics** or dynamic subscriptions from a single function.

If you only need to consume a topic and handle each record, prefer the runtime —
opting out means taking on everything below yourself.

## What you take on

Going direct means the function is a normal function with a Kafka library embedded;
`func` injects nothing and knows nothing about Kafka. You own:

- **The client lifecycle** — connecting, consumer-group membership, rebalancing,
  retries, and offset commits.
- **Delivery semantics** — there is no managed at-least-once wrapper; your code
  decides when a record is "done."
- **Scaling** — the managed path offers a `kafka` consumer-lag trigger, but even
  there you declare it explicitly under `scale.keda.triggers` with the `keda`
  deployer (it is never inferred from `run.kafka`). Going direct, you configure KEDA
  (or another autoscaler) yourself, or run a fixed replica count.
- **Credentials** — you supply broker addresses and TLS/SASL config to your client
  through environment variables, mounted Secrets, etc.
- **Language specificity** — you now depend on a Kafka client for your function's
  language, so the function is no longer language-agnostic.

## How it looks

Do **not** set `run.kafka` (that would also inject the sidecar — don't run both
against the same topic/group). Instead, add a Kafka client dependency and consume
in your function, for example conceptually:

```text
on startup:
    consumer = KafkaConsumer(brokers = env.BROKERS,
                             group    = env.CONSUMER_GROUP,
                             ...tls/sasl from env/secrets)
    consumer.subscribe(env.TOPIC)

loop:
    records = consumer.poll()
    for record in records:
        process(record)
    consumer.commit()        # your chosen offset strategy
```

Deploy it like any other function, providing the broker configuration and
credentials through environment variables and mounted Secrets, and set up scaling
to suit your workload.

## Related

- [architecture.md](architecture.md) — what the runtime does, so you know exactly
  what you're replacing.
- [deployment-and-scaling.md](deployment-and-scaling.md) — how the managed path
  wires scaling, if you want to mirror it.
