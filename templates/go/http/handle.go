package function

import (
	"context"
	"net/http"
)

// Handle an HTTP Request.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) error {
	res.Header().Add("Content-Type", "text/plain")
	res.Write([]byte("OK\n"))
	return nil
}
