package function

import (
	"context"
	"fmt"
	"net/http"
)

// Handle an HTTP Request.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	// Example implementation:
	fmt.Println("OK")       // Print "OK" to standard output (local logs)
	fmt.Fprintln(res, "OK") // Send "OK" back to the client
}
