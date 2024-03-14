package function

import (
	"fmt"
	"net/http"
	"net/http/httputil"
)

// Handle an HTTP Request.
func Handle(w http.ResponseWriter, r *http.Request) {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("Received request")
	fmt.Printf("%q\n", dump)
	fmt.Fprintf(w, "%q", dump)
}
