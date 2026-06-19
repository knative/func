//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// TestInstanced_GoHTTP deploys a Go HTTP function using the instanced
// signature and verifies that constructor state persists across requests.
func TestInstanced_GoHTTP(t *testing.T) {
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

	if !waitFor(t, ksvcUrl(name), withContentMatch("request:1")) {
		t.Fatal("instanced Go HTTP function did not return request:1")
	}

	body := sendRequest(t, ksvcUrl(name))
	if !strings.Contains(body, "request:2") {
		t.Fatalf("expected request:2 on second call, got: %s", body)
	}
}

// TestInstanced_PythonHTTP deploys a Python HTTP function using the instanced
// signature and verifies constructor state persists across requests.
func TestInstanced_PythonHTTP(t *testing.T) {
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

	if !waitFor(t, ksvcUrl(name), withContentMatch("request:1")) {
		t.Fatal("instanced Python HTTP function did not return request:1")
	}

	body := sendRequest(t, ksvcUrl(name))
	if !strings.Contains(body, "request:2") {
		t.Fatalf("expected request:2 on second call, got: %s", body)
	}
}
