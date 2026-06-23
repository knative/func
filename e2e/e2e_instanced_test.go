//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// INSTANCED SIGNATURE TESTS
// Dedicated tests proving the instanced function pattern (New() constructor +
// struct-based Handle) works end-to-end for Go and Python. Each test deploys
// a function with a counter in the constructor, sends two requests, and
// asserts the counter increments — proving the instance persists across
// requests.
// ---------------------------------------------------------------------------

const goHTTPInstancedSource = `package function

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Function struct{ count int64 }

func New() *Function { return &Function{} }

func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	n := atomic.AddInt64(&f.count, 1)
	fmt.Fprintf(w, "request:%d", n)
}
`

const pythonHTTPInstancedSource = `def new():
    return Function()

class Function:
    def __init__(self):
        self.count = 0

    async def handle(self, scope, receive, send):
        self.count += 1
        body = f"request:{self.count}".encode()
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [[b'content-type', b'text/plain']],
        })
        await send({
            'type': 'http.response.body',
            'body': body,
        })
`

// sendRequest performs a single HTTP GET and returns the body as a string.
func sendRequest(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("sendRequest GET %s: %v", url, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("sendRequest reading body: %v", err)
	}
	return string(body)
}

// requestCount issues one GET and parses the integer N from a "request:N" body.
func requestCount(t *testing.T, url string) int {
	t.Helper()
	body := sendRequest(t, url)
	_, num, ok := strings.Cut(body, "request:")
	if !ok {
		t.Fatalf("unexpected response (missing \"request:\" prefix): %q", body)
	}
	n, err := strconv.Atoi(strings.TrimSpace(num))
	if err != nil {
		t.Fatalf("could not parse counter from response %q: %v", body, err)
	}
	return n
}

// TestCore_InstancedGoHTTP deploys a Go HTTP function using the instanced
// signature and verifies that constructor state persists across requests.
func TestCore_InstancedGoHTTP(t *testing.T) {
	name := "func-e2e-instanced-go-http"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "function.go"), []byte(goHTTPInstancedSource), 0644); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy", "--builder", "host").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// Wait for readiness by matching the "request:" PREFIX
	if !waitFor(t, ksvcUrl(name), withContentMatch("request:")) {
		t.Fatal("instanced HTTP function did not become ready")
	}

	// Dont check for exact match because we poll function first to check if
	// its live + some health checks send requests too, this is simpler
	first := requestCount(t, ksvcUrl(name))
	second := requestCount(t, ksvcUrl(name))
	if second <= first {
		t.Fatalf("instanced counter did not increase across requests (state should persist): first=%d second=%d", first, second)
	}
}

// TestCore_InstancedPythonHTTP deploys a Python HTTP function using the instanced
// signature and verifies constructor state persists across requests.
func TestCore_InstancedPythonHTTP(t *testing.T) {
	name := "func-e2e-instanced-py-http"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=python", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "function", "func.py"), []byte(pythonHTTPInstancedSource), 0644); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy", "--builder", "host").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name), withContentMatch("request:")) {
		t.Fatal("instanced HTTP function did not become ready")
	}

	first := requestCount(t, ksvcUrl(name))
	second := requestCount(t, ksvcUrl(name))
	if second <= first {
		t.Fatalf("instanced counter did not increase across requests (state should persist): first=%d second=%d", first, second)
	}
}
