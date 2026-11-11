# Idempotency & deduplication

Kafka functions are delivered [at-least-once](delivery-and-retries.md), so your
handler can occasionally receive the **same record more than once**. Making the
handler safe to run twice for the same record is the function author's
responsibility. This page explains when duplicates happen and two patterns for
handling them.

## Why duplicates happen

The sidecar commits a record's offset only after your function returns a 2xx. A
duplicate occurs whenever work completes but the acknowledgement is lost before the
offset is committed, for example:

- The function does its work and returns 2xx, but the pod crashes (or is scaled
  down) before the sidecar commits the offset. After restart, consumption resumes
  from the last committed offset and the record is delivered again.
- A consumer-group rebalance (a replica added or removed) reassigns a partition;
  any records processed but not yet committed are redelivered to the new owner.
- A response is slow enough to hit the per-attempt timeout, so the attempt is
  retried even though the first one actually succeeded on the function side.

None of these lose data — that is the point of at-least-once — but they do mean
"processed exactly once" is something your handler must arrange, not something the
transport guarantees.

## Choosing a deduplication key

Pick an identifier that is **stable across redeliveries** of the same record:

- **The record's Kafka coordinates.** For a plain record the runtime sets the
  CloudEvent `id` to `partition:<p>/offset:<o>` and also exposes `kafkatopic`,
  `kafkapartition`, and `kafkaoffset` extensions. That `id` is stable across
  redeliveries, but it is built from partition and offset **only** — it is *not*
  unique across topics, nor across a topic that is deleted and re-created (offsets
  restart at `0`). So do **not** key on `event.id` alone if your store is shared by
  more than one function or topic: compose a key from the **consumer group** plus
  `kafkatopic`, `kafkapartition`, and `kafkaoffset`. (A record passed through as the
  producer's *own* CloudEvent — see [architecture.md](architecture.md#record--cloudevent-mapping)
  — carries the producer's `id` and **no** `kafka*` extensions; key on that `id`
  instead for those events.)
- **A business key from the payload** — an order ID, a transaction ID, etc. Use
  this when you want to deduplicate by *meaning* (the same order submitted twice is
  one order) rather than by record.

Avoid the Kafka message *key* as a dedup key: it identifies a partitioning group,
not an individual record, and is usually shared by many records.

## Pattern 1: a deduplication store

Keep a record of keys you have already processed in a shared store (Redis, a
database table, etc.) and skip anything you have already **finished**. The subtlety
is that a crash *between* claiming a key and finishing the work must not leave the
key marked as done — otherwise the redelivery that would have retried it is acked
and the record is silently lost. Use a two-phase claim: take a short-lived **lease**
before the work, and record completion only **after** it succeeds:

```text
function handle(event):
    # The CloudEvent id is partition:offset only, so it is not unique across
    # topics or functions sharing this store. Compose a key from the consumer
    # group + the record's Kafka coordinates (see "Choosing a deduplication key").
    key = consumerGroup + "/" + event.kafkatopic + "/" \
          + event.kafkapartition + "/" + event.kafkaoffset

    # Fast path: this key was already processed to completion.
    if store.isDone(key):
        return 200            # already handled — ack, do nothing

    # Atomically take a short-lived lease so two replicas can't process the
    # same redelivery at once. The lease EXPIRES on its own, so a crash before
    # markDone does not lose the record: a later redelivery re-acquires the
    # expired lease and retries the work.
    if not store.acquireLease(key, leaseTtl):
        return 409            # another replica holds the lease — let Kafka redeliver

    process(event)            # your business logic
    store.markDone(key, ttl)  # record completion only AFTER the work succeeds
    return 200
```

Recording completion only *after* the work succeeds is what makes this safe: an
interrupted attempt leaves no "done" marker, so at-least-once redelivery still
retries it. The lease closes the concurrent-duplicate race (two replicas, or a
retry overlapping a slow first attempt) without turning an in-flight claim into a
permanent acknowledgement. A plain set-if-absent that acks on the first sight of a
key *looks* simpler but drops the record whenever the handler dies after the claim
and before the work — prefer [Pattern 2](#pattern-2-idempotent-writes) when your
sink allows it, and reach for this lease only for non-idempotent side effects.

## Pattern 2: idempotent writes

Often the simplest approach is to make the side effect itself idempotent, so
reprocessing is harmless and no separate dedup store is needed. The classic case is
an upsert keyed by a natural identifier:

```text
function handle(event):
    order = parse(event)
    db.upsert(key = order.id, value = order)   # insert-or-replace, not append
    return 200
```

Writing the same row twice leaves the same result. This works whenever your sink
supports upserts or conditional writes; prefer it over a dedup store when you can.

## Pitfalls

- **Check-then-act races.** A non-atomic "is it seen? → process → mark seen" can
  double-process under concurrency (two replicas, or a retry overlapping a slow
  first attempt). Take a lease atomically before processing and mark the key done
  only after the work succeeds, as in Pattern 1.
- **Acking an in-flight claim.** Marking a key "done" *before* the work finishes
  means a crash in between acks a record that was never processed — a silent drop.
  Record completion only after the work succeeds (Pattern 1's lease), or make the
  write itself idempotent (Pattern 2).
- **Acknowledge duplicates with 2xx.** When you detect a duplicate, return a
  success so the offset advances. Returning an error would retry the record
  forever.
- **Store retention.** A dedup store needs a retention/TTL policy long enough to
  outlast realistic redelivery windows (restarts, rebalances, outages). Too short
  and a late redelivery slips through; unbounded and the store grows forever.
- **Non-idempotent side effects.** Sending an email or calling a non-idempotent API
  can't be "undone" by an upsert — guard those with Pattern 1, or make the
  downstream call idempotent (e.g. an idempotency key on the API request).

## Related

- [delivery-and-retries.md](delivery-and-retries.md) — why at-least-once implies duplicates.
- [architecture.md](architecture.md) — how records map to CloudEvents.
