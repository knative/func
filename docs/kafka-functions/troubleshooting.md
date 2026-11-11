# Troubleshooting

Common problems with Kafka functions and how to diagnose them. A Kafka function
runs **two containers** in one pod — your function (`user-container`) and the
injected Kafka runtime (`kafka-runtime`) — so most diagnosis starts by deciding
which one is unhappy.

## Where to look first

```bash
# The injected sidecar's logs — Kafka connection, consumption, delivery results.
kubectl logs deployment/<function> -c kafka-runtime

# Your function's logs — whether it actually received the CloudEvents.
kubectl logs deployment/<function> -c user-container

# Consumer-group lag per partition (is the function keeping up?).
kubectl run kafka-lag --rm -i --restart=Never -n kafka \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-consumer-groups.sh --bootstrap-server <broker> \
  --describe --group <consumerGroup>
```

The sidecar serves its own health endpoint on port `8081`; if the pod never
becomes Ready, `kubectl describe pod <pod>` shows which probe is failing.

## Deploy never becomes Ready

The sidecar must connect to the broker and be assigned partitions before it
reports healthy, so a stuck deploy usually means it can't reach Kafka:

- **Wrong broker address.** `brokers` must be resolvable *from inside the
  cluster* — use the in-cluster service name (e.g.
  `my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9092`), not a localhost or
  external address.
- **Network policy / firewall** blocking the broker port.
- **Bad credentials or security settings** — see the auth section below.

Check `kubectl logs deployment/<function> -c kafka-runtime` for the connection
error, and `kubectl describe pod` for probe failures.

## The function receives nothing, but the topic has messages

The runtime consumes from the **newest** offset, so a brand-new consumer group
only sees records produced **after** it joins. Records sent before the consumer
was ready are skipped, not replayed.

- Produce **after** the sidecar logs that it is ready (partitions assigned), then
  check the function logs.
- Double-check `topic` and `consumerGroup` match what you're producing to.
- Confirm the broker actually has the records (end offsets should be non-zero).

## Authentication or TLS failures

Symptoms are connection errors in the sidecar log at startup:

- **`securityProtocol` mismatch** — e.g. using `PLAINTEXT` against a TLS listener,
  or `SASL_SSL` against a plaintext one. Match the protocol to the broker's
  listener.
- **Wrong SASL mechanism** — the broker expects a specific mechanism (`PLAIN`,
  `SCRAM-SHA-256`, `SCRAM-SHA-512`); set `sasl.mechanism` accordingly.
- **Missing or wrong CA certificate** — for `SSL`/`SASL_SSL` with a private CA,
  `tls.caCert` must point at the mounted CA path and the Secret must exist in the
  function's namespace.
- **Credentials not found** — a `{{ secret:name:key }}` reference must name a
  Secret (and key) that exists in the namespace before you deploy.

## Lag keeps growing / a partition is stuck

If lag climbs and never drains:

- **The function keeps failing.** With the default **unlimited** retries, a record
  the function never accepts is retried forever and its partition does not advance
  — which preserves ordering but blocks that partition. Fix the function, or set a
  bounded `delivery.retry` so an unprocessable "poison" record is eventually
  skipped. See [delivery-and-retries.md](delivery-and-retries.md).
- **Scaled to zero with the wrong trigger.** If the function uses an `http` trigger
  (or the default) with `scale.min: 0`, it scales to zero when there's no HTTP
  traffic and then nothing wakes it to drain Kafka lag. Use a `kafka` trigger or
  set `scale.min: 1`. See [deployment-and-scaling.md](deployment-and-scaling.md).
- **Not enough consumers.** At most one replica per partition does useful work; if
  lag is high and `scale.max` is below the partition count, you can't parallelize
  further.

## The function sees the same record twice

This is expected: delivery is at-least-once, so a record can be redelivered after a
crash, a rebalance, or a retry whose success response was lost. Make the handler
idempotent — see [idempotency.md](idempotency.md).

## Sidecar `ImagePullBackOff`

The cluster can't pull the runtime image. Confirm the image is reachable from the
cluster, and if you overrode it with `FUNC_KAFKA_RUNTIME_IMAGE`, that the override
name is correct and the registry is accessible.

## Confirming the configuration reached the sidecar

The Kafka and delivery settings are carried by the **sidecar**, not the function
container. To confirm what was actually deployed:

```bash
kubectl get deployment <function> -o json \
  | jq '.spec.template.spec.containers[] | select(.name=="kafka-runtime").env'
```

You should see the `KAFKA_*` connection variables and, if you tuned delivery, the
`FUNCTION_DELIVERY_*` / `FUNCTION_REQUEST_TIMEOUT` variables. The function
container stays plain — it carries none of these.
