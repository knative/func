# Delivery, retries & offsets

This page explains *how* records reach your function and *what happens when
delivery fails*, so you can choose a retry policy that fits your workload. For the
exact field names, types, and defaults, see the
[`func.yaml` reference](../reference/func_yaml.md#delivery-tuning); this page is
about behavior.

## At-least-once delivery

The runtime sidecar consumes a record, converts it to a CloudEvent, and POSTs it
to your function over localhost. It commits the record's Kafka offset **only after
your function responds with a 2xx**. Anything else — a non-2xx response, a
per-attempt timeout, or a dropped connection — counts as a failed attempt, and the
offset is **not** advanced.

Because the offset moves forward only on success, no record is lost: a record is
redelivered until it is accepted or its retry budget runs out. The flip side is
that your function can occasionally see the **same record more than once** (for
example, if it did its work but crashed before the 2xx was sent). This is the
standard at-least-once trade-off — design your handler to tolerate duplicates. See
[idempotency.md](idempotency.md).

## What happens when an attempt fails

A failed attempt is retried after a backoff delay. The `delivery` block tunes the
budget, the spacing, and the per-attempt timeout:

```yaml
run:
  kafka:
    brokers: my-cluster-kafka-bootstrap:9093
    topic: orders
    consumerGroup: myfunc
    delivery:
      retry: 5
      backoffPolicy: exponential
      backoffDelay: 200ms
      backoffMaxDelay: 10s
      timeout: 30s
```

With **no `delivery` block**, the defaults apply: **unlimited retries**,
**exponential** backoff from **100ms** up to a **30s** cap, and a **30s**
per-attempt timeout.

### Retry budget (`retry`)

`retry` counts attempts *after* the first one, so total attempts = `retry` + 1.

- **Omitted → unlimited.** The record is retried forever until the function
  accepts it. Nothing is ever dropped. This is the default.
- **A number N →** up to `N + 1` attempts, then the budget is exhausted (see
  below). `retry: 5` means six attempts in total.
- **`0` →** a single attempt with no retry. On failure the budget is immediately
  exhausted.

### Backoff (`backoffPolicy`, `backoffDelay`, `backoffMaxDelay`)

Between attempts the runtime waits, growing the delay from `backoffDelay` (base)
up to `backoffMaxDelay` (cap):

- **`exponential`** (default) — the delay roughly doubles each attempt until it
  reaches the cap: 100ms, 200ms, 400ms, … 30s, 30s, …
- **`linear`** — the delay grows by the base each attempt: 100ms, 200ms, 300ms, …
  up to the cap.

The cap keeps a long-running retry loop from waiting arbitrarily long between
attempts.

### Per-attempt timeout (`timeout`)

`timeout` bounds a **single** delivery attempt. If the function does not respond
within it, the runtime cancels that attempt and treats it as a failure — which
then counts against the retry budget and triggers backoff like any other failure.
It is not a total deadline across all retries.

## Exhaustion: what a bounded budget does

With the default unlimited budget there is no exhaustion — a persistently failing
record simply keeps being retried.

When you set a **finite** `retry` and it runs out, the runtime **skips the record
and commits its offset**, emitting a loud warning log. The partition then moves on
to the next record. This is deliberate: a finite budget is how you prevent a
single unprocessable record (a "poison pill") from wedging a partition forever.

> Skipping means the record is **dropped** — it is neither processed nor stored
> anywhere. Dead-letter routing (sending exhausted records to another topic
> instead of dropping them) is planned but not yet available.

## Choosing a policy

The choice is between never losing a record and never getting stuck:

- **Unlimited retries (default)** — nothing is ever dropped, and within a
  partition ordering is preserved because the runtime does not advance past a
  record it hasn't delivered. The risk: a truly unprocessable record blocks its
  partition until the function is fixed or the record ages out of the topic.
- **Bounded retries** — a poison pill is eventually skipped so the partition keeps
  flowing, at the cost of dropping that record. Choose this when steady progress
  matters more than processing every single record, and pair it with monitoring so
  you notice the warning logs.

A middle ground is a generous budget (say `retry: 20` with exponential backoff) so
transient outages are ridden out but a permanently bad record is eventually
skipped.

## Validation

The delivery settings are validated at **deploy time**, so mistakes fail fast
rather than misbehaving on the cluster. Rejected configurations include a negative
`retry`, an unknown `backoffPolicy`, an unparseable duration, and a
`backoffMaxDelay` smaller than the effective `backoffDelay` (the check accounts for
defaults when only one side is set). See the
[`func.yaml` reference](../reference/func_yaml.md#delivery-tuning) for the exact
rules.

## Next steps

- [idempotency.md](idempotency.md) — handling the duplicates at-least-once implies.
- [deployment-and-scaling.md](deployment-and-scaling.md) — how consumers scale on lag.
- [architecture.md](architecture.md) — the record → CloudEvent → offset-commit flow.
