package function

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandle ensures that the constructor returns an object which handles
func TestHandle(t *testing.T) {
	var (
		w   = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "http://example.com/test", nil)
		res *http.Response
	)

	New().Handle(w, req)
	res = w.Result()
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("unexpected response code: %v", res.StatusCode)
	}
}
