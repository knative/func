//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCore_PythonUpdate verifies that redeploying a Python function after
// changing its source code actually serves the new code.
// Regression test for issue #3079.
func TestCore_PythonUpdate(t *testing.T) {
	name := "func-e2e-test-python-update"
	root := fromCleanEnv(t, name)

	// create
	if err := newCmd(t, "init", "-l=python", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy", "--builder", "pack").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute)) {
		t.Fatal("function did not deploy correctly")
	}

	// update: rewrite func.py with a new response body
	updated := `import logging

def new():
    return Function()

class Function:
    async def handle(self, scope, receive, send):
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [
                [b'content-type', b'text/plain'],
            ],
        })
        await send({
            'type': 'http.response.body',
            'body': b'UPDATED',
        })
`
	err := os.WriteFile(filepath.Join(root, "function", "func.py"), []byte(updated), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// redeploy
	if err := newCmd(t, "deploy", "--builder", "pack").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute),
		withContentMatch("UPDATED")) {
		t.Fatal("function did not update correctly (issue #3079: poetry-venv cache not invalidated on source change)")
	}
}
