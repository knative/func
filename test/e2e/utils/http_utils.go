package utils

import (
	"io/ioutil"
	"net/http"
	"testing"
)

// HttpGet Convinient wrapper that calls an URL and returns just the
// body and status code. It fails in case some error occurs in the call
func HttpGet(t *testing.T, url string) (body string, statusCode int) {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Error returned calling %v : %v", url, err.Error())
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err.Error())
	}
	statusCode = resp.StatusCode
	t.Logf("Called GET %v -> %v", url, resp.Status)
	body = string(b)
	t.Log(body)
	return body, statusCode
}