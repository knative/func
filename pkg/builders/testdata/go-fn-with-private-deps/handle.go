package function

import (
	"fmt"
	"net/http"

	"git-private.127.0.0.1.sslip.io/foo.git/pkg/foo"
)

// Handle an HTTP Request.
func Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")
	w.WriteHeader(200)
	_, _ = fmt.Fprintf(w, "The answer is: %d", foo.Foo())
}
