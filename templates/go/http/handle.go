package function

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

// Handle an HTTP Request.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {

	res.Header().Add("Content-Type", "text/plain")
	res.Header().Add("Content-Length", "3")
	res.WriteHeader(200)

	_, err := fmt.Fprintf(res, "OK\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error or response write: %v", err)
	}
}
