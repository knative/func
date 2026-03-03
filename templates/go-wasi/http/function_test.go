package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handle(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Hello from WASI!") {
		t.Errorf("unexpected body: %q", body)
	}
	if !strings.Contains(body, "Path: /") {
		t.Errorf("body missing path, got: %q", body)
	}
}

func TestHandleCustomPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/greet?name=world", nil)
	w := httptest.NewRecorder()

	handle(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Path: /greet") {
		t.Errorf("body missing custom path, got: %q", body)
	}
}
