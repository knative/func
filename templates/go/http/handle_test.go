package function

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandle ensures that Handle executes without error and returns the
// HTTP 200 status code indicating no errors.
func TestHandle(t *testing.T) {
	var (
		w   = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "http://example.com/test", nil)
		res *http.Response
		err error
	)

	// Invoke the Handler via a standard Go http.Handler
	func(w http.ResponseWriter, req *http.Request) {
		Handle(context.Background(), w, req)
	}(w, req)

	res = w.Result()
	defer res.Body.Close()

	// Assert postconditions
	if err != nil {
		t.Fatalf("unepected error in Handle: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("unexpected response code: %v", res.StatusCode)
	}
}
