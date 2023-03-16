package testhttp

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestGet(t *testing.T, url string) (statusCode int, body string) {
	return TestUrl(t, "GET", "", url, nil)
}

func TestUrl(t *testing.T, method string, bodyData string, url string, headers url.Values) (statusCode int, body string) {
	req, err := http.NewRequest(method, url, strings.NewReader(bodyData))
	assert.NilError(t, err)

	for k, v := range headers {
		for _, hv := range v {
			req.Header.Add(k, hv)
		}
	}

	client := &http.Client{Timeout: time.Second * 15}
	resp, err := client.Do(req)

	t.Logf("%s %v -> %v", method, url, resp.Status)
	assert.NilError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	message, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)

	statusCode = resp.StatusCode
	body = string(message)
	return
}

type TestHeaders struct {
	Headers url.Values
}

func HeaderBuilder() *TestHeaders {
	h := make(url.Values)
	return &TestHeaders{h}
}

func (t *TestHeaders) Add(header string, value string) *TestHeaders {
	t.Headers.Add(header, value)
	return t
}

func (t *TestHeaders) AddNonEmpty(header string, value string) *TestHeaders {
	if value != "" {
		t.Headers.Add(header, value)
	}
	return t
}
