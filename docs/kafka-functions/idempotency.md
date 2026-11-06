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

- **The CloudEvent `id`.** The runtime assigns each record a CloudEvent id that is
  the same every time that record is redelivered, which makes it a natural
  deduplication key with no assumptions about your payload.
- **A business key from the payload** — an order ID, a transaction ID, etc. Use
  this when you want to deduplicate by *meaning* (the same order submitted twice is
  one order) rather than by record.

Avoid the Kafka message *key* as a dedup key: it identifies a partitioning group,
not an individual record, and is usually shared by many records.

## Pattern 1: a deduplication store

Keep a record of keys you have already processed in a shared store (Redis, a
database table, etc.) and skip anything you have seen before. Do the "claim"
**atomically and before** the work, so two replicas handling the same redelivery
cannot both proceed:

```text
function handle(event):
    key = event.id

    # Atomic "set if absent": returns false if the key already existed.
    if not store.add(key, ttl):
        return 200            # already handled (or in flight) — ack, do nothing

    process(event)           # your business logic
    return 200
```

Using an atomic set-if-absent (rather than a separate `seen?` check followed by a
`mark`) closes the race where two concurrent deliveries both read "not seen" and
both process the record.

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
  first attempt). Claim the key atomically before processing, as in Pattern 1.
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
