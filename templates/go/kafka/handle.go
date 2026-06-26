package function

import (
	"context"
	"fmt"

	"knative.dev/func-go/kafka"
)

// Handle a Kafka message.
//
// Returning nil signals successful processing and the message offset is
// committed.  Returning an error logs the error and the message is skipped
// (not retried).
func Handle(ctx context.Context, msg kafka.Message) error {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	fmt.Printf("Received message: topic=%s partition=%d offset=%d key=%s value=%s\n",
		msg.Topic, msg.Partition, msg.Offset, string(msg.Key), string(msg.Value))
	return nil
}
