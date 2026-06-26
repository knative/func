# Go Kafka Function

Welcome to your new Go Function! The boilerplate function code can be found in
[`handle.go`](handle.go). This Function consumes messages from Kafka topics.

## How it works

Your `Handle` function is called once for each Kafka message. Return `nil` to
indicate successful processing — the message offset will be committed
automatically. Return an error to skip the message (the error is logged and
the consumer moves on to the next message).

## Delivery guarantees

Messages are delivered **at-least-once** and processed **in order per
partition**. If the consumer crashes after processing a message but before the
offset is committed, the message will be redelivered. There is no built-in
deduplication — if your function cannot safely process the same message twice,
you should implement idempotency in your `Handle` function (for example by
tracking previously seen message keys or offsets).

## Development

Develop new features by adding a test to [`handle_test.go`](handle_test.go) for
each feature, and confirm it works with `go test`.

Update the running analog of the function using the `func` CLI or client
library. The function will consume messages from the Kafka topics configured
via the `KAFKA_TOPICS` environment variable.

For more, see [the complete documentation]('https://github.com/knative/func/tree/main/docs')

