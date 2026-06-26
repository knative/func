# Kafka Function Instance

Welcome to your new Go Function! The boilerplate function code can be found in
[`function.go`](function.go). This Function consumes messages from Kafka topics.

## How it works

Your `Handle` method is called once for each Kafka message. Return `nil` to
indicate successful processing — the message offset will be committed
automatically. Return an error to skip the message (the error is logged and
the consumer moves on to the next message).

## Delivery guarantees

Messages are delivered **at-least-once** and processed **in order per
partition**. If the consumer crashes after processing a message but before the
offset is committed, the message will be redelivered. There is no built-in
deduplication — if your function cannot safely process the same message twice,
you should implement idempotency in your `Handle` method (for example by
tracking previously seen message keys or offsets).

## Development

Develop new features by adding a test to [`function_test.go`](function_test.go) for
each feature, and confirm it works with `go test`.

Once your function is passing tests, deploy it using `func deploy`.  The
`func` CLI also offers several other testing and development commands; see
`func --help` for more.

For more, see [the complete documentation]('https://github.com/knative/func/tree/main/docs')

