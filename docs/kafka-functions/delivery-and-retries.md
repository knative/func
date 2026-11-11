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

Because the offset moves forward only on success, a record is redelivered until it
is accepted — with the default unlimited budget it is never lost. (The one exception
is a **finite** `retry` budget: when it is exhausted the runtime deliberately skips
and commits the record — see [Exhaustion](#exhaustion-what-a-bounded-budget-does).)
The flip side is
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
- **`0` →** a single attempt with no retry. On a rejection the budget is
  immediately exhausted.

### What counts toward the budget

**Every** failed attempt counts, regardless of why it failed:

- **A non-2xx response** — the function got the record and rejected it (a "poison
  pill").
- **A per-attempt timeout** — the function did not respond within `timeout`.
- **A transport error** — the function was unreachable (a dropped connection, for
  example during a rollout, crash, or scale-up).

The runtime does not try to tell these apart: a failed delivery is a failed
delivery. The one thing that is *not* a failed attempt is the consumer shutting
down or rebalancing mid-delivery — that record is simply redelivered in the next
session, it does not spend the budget.

This matches how a finite budget behaves elsewhere (for example Knative Eventing),
and it keeps the exhaustion rule simple: `retry: N` means "try at most `N + 1`
times, then give up," no matter the failure. To ride out restarts and transient
outages without ever dropping a record, use the **unlimited** default (see
[Choosing a policy](#choosing-a-policy)).

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
within it, the runtime cancels that attempt and treats it as a failure: the offset
is not advanced and backoff is applied before the next attempt. A timeout is a
failed attempt like any other, so it consumes the finite retry budget (see [What
counts toward the budget](#what-counts-toward-the-budget)). It is not a total
deadline across all retries.

## Exhaustion: what a bounded budget does

With the default unlimited budget there is no exhaustion — a persistently failing
record simply keeps being retried.

When you set a **finite** `retry` and a record fails all of its attempts — the
initial one plus `retry` retries, i.e. `retry + 1` in total, for any reason (see
[What counts toward the budget](#what-counts-toward-the-budget)) — the runtime
**skips the record and commits its offset**, emitting a loud warning log.
The partition then moves on to the next record. This is deliberate: a finite budget
is how you prevent a single unprocessable record (a "poison pill") from wedging a
partition forever. The trade-off is that a sustained outage longer than the budget
can also exhaust it and drop a good record — which is why the default is unlimited
retries (see [Choosing a policy](#choosing-a-policy)).

> Skipping means the record is **dropped** — it is neither processed nor stored
> anywhere. Dead-letter routing (sending exhausted records to another topic
> instead of dropping them) is planned but not yet available.

## Choosing a policy

The choice is between never losing a record and never getting stuck:

- **Unlimited retries (default)** — nothing is ever dropped, and within a
  partition ordering is preserved because the runtime does not advance past a
  record it hasn't delivered. Restarts and transient outages are always ridden out.
  The risk: a truly unprocessable record blocks its partition until the function is
  fixed. The runtime holds the record and keeps retrying it from memory — it never
  re-reads the log — so topic retention does **not** clear a stuck partition on a
  running sidecar. Worse, if the sidecar restarts or the partition rebalances while
  stuck and the record has by then aged out of the topic, its committed offset is
  below the earliest offset still retained; the consumer treats that as out-of-range
  and resets to the **newest** offset, silently skipping every record produced in
  the meantime — not just the poison record. Fix the handler (or use a bounded
  budget); do not rely on retention to unblock a partition.
- **Bounded retries** — a record that keeps failing is eventually skipped so the
  partition keeps flowing, at the cost of dropping that record. Choose this when
  steady progress matters more than processing every single record, and pair it with
  monitoring so you notice the warning logs.

With a finite budget, every failure counts — including an outage while the function
is down — so size `retry` for the longest interruption you are willing to ride out,
not just for how many times you want to re-offer a poison record. Because your
backoff grows up to `backoffMaxDelay`, a handful of retries already spans a fair bit
of wall-clock time; still, a short outage can exhaust a small budget. If you cannot
tolerate dropping a record during a restart, keep the unlimited default. Dead-letter
routing (sending exhausted records elsewhere instead of dropping them) is planned
and will remove this trade-off.

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
