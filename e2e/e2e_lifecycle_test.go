//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// describe how to set up a Function for a specific runtimes testing lifecycle hooks
type lifecycleRuntime struct {
	builder string // "host", "pack", "s2i"
	// TODO: expand matrix for each deployer
	// deployer string   // "knative","raw","keda"
	srcPath  string   // where to write source in func dir (e.g. "function.go")
	srcExt   string   // extension for testdata lookup (".go", ".py")
	initArgs []string // extra args for func init
}

var allRuntimes = []string{
	"go", "python", "node", "typescript", "rust", "quarkus", "springboot",
}

var lifecycleRuntimes = map[string]*lifecycleRuntime{
	"go": {
		builder:  "host",
		srcPath:  "function.go",
		srcExt:   ".go",
		initArgs: []string{},
	},
	"python": {
		builder:  "host",
		srcPath:  filepath.Join("function", "func.py"),
		srcExt:   ".py",
		initArgs: []string{},
	},
}

// loads testdata/lifecycle/{runtime}/{hook}{extension}
// lifecycleSource reads the testdata source file for the given runtime and hook.
func lifecycleSource(t *testing.T, runtime, hook string, rt *lifecycleRuntime) []byte {
	t.Helper()
	path := filepath.Join(Testdata, "lifecycle", runtime, hook+rt.srcExt)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lifecycleSource: reading %s: %v", path, err)
	}
	return data
}

// getBody performs a single HTTP GET and returns the response body as a string.
func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("getBody: GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("getBody: reading body from %s: %v", url, err)
	}
	return string(body)
}

// extractValue parses KEY=value pairs from a space-separated response body and
// returns the value for the given key, or "" if the key is not present.
// Example: extractValue("ALIVE=true NONCE=12345", "NONCE") == "12345"
func extractValue(body, key string) string {
	for _, field := range strings.Fields(body) {
		if k, v, ok := strings.Cut(field, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// TestLifecycle_Hooks verifies the Start, Ready, and Alive hooks using a single
// merged function per runtime. The function implements all three hooks
// simultaneously, and the test verifies them in sequence:
//  1. Start — fires during cold start, captures config env var
//  2. Ready — returns false for 15s warm-up, then true
//  3. Alive — defaults true, toggled false via /set-unhealthy, pod restarts
func TestLifecycle_Hooks(t *testing.T) {
	for _, name := range allRuntimes {
		rt, enabled := lifecycleRuntimes[name]
		t.Run(name, func(t *testing.T) {
			if !enabled {
				t.Skipf("lifecycle hooks not enabled for %s", name)
			}

			funcName := fmt.Sprintf("func-e2e-test-lifecycle-hooks-%s", name)
			root := fromCleanEnv(t, funcName)

			args := append([]string{"init", "-l=" + name}, rt.initArgs...)
			if err := newCmd(t, args...).Run(); err != nil {
				t.Fatal(err)
			}

			if err := newCmd(t, "config", "envs", "add",
				"--name=TEST_CONFIG_VALUE", "--value=e2e-lifecycle-hooks").Run(); err != nil {
				t.Fatal(err)
			}

			src := lifecycleSource(t, name, "hooks", rt)
			if err := os.WriteFile(filepath.Join(root, rt.srcPath), src, 0644); err != nil {
				t.Fatal(err)
			}

			deployArgs := []string{"deploy"}
			if rt.builder != "" {
				deployArgs = append(deployArgs, "--builder", rt.builder)
			}
			if err := newCmd(t, deployArgs...).Run(); err != nil {
				t.Fatal(err)
			}
			defer clean(t, funcName, Namespace)
			if rt.builder != "" {
				defer cleanImages(t, funcName)
			}

			url := ksvcUrl(funcName)

			// Phase 1: Start hook — verify it fired and received config
			t.Run("start", func(t *testing.T) {
				if !waitFor(t, url,
					withContentMatch("START_OK:e2e-lifecycle-hooks"),
					withWaitTimeout(5*time.Minute)) {
					t.Fatal("Start hook did not fire or did not receive config")
				}
			})

			// Phase 2: Ready hook — verify readiness endpoint
			t.Run("ready", func(t *testing.T) {
				if !waitFor(t, url,
					withContentMatch("READY=true"),
					withWaitTimeout(5*time.Minute)) {
					t.Fatal("Ready hook: function never became ready")
				}

				body := getBody(t, url+"/health/readiness")
				if !strings.Contains(strings.ToUpper(body), "READY") {
					t.Fatalf("readiness endpoint returned unexpected body: %s", body)
				}
			})

			// Phase 3: Alive hook — toggle unhealthy, verify pod restart via nonce
			t.Run("alive", func(t *testing.T) {
				body := getBody(t, url)
				nonce1 := extractValue(body, "NONCE")
				if nonce1 == "" {
					t.Fatalf("no NONCE in response: %s", body)
				}
				t.Logf("initial nonce: %s", nonce1)

				unhealthyBody := getBody(t, url+"/set-unhealthy")
				t.Logf("set-unhealthy response: %s", unhealthyBody)

				if !waitFor(t, url,
					withContentMatch("ALIVE=true"),
					withWaitTimeout(3*time.Minute)) {
					t.Fatal("function did not recover after liveness failure")
				}

				body = getBody(t, url)
				nonce2 := extractValue(body, "NONCE")
				if nonce2 == "" {
					t.Fatalf("no NONCE in response after restart: %s", body)
				}
				t.Logf("post-restart nonce: %s", nonce2)

				if nonce1 == nonce2 {
					t.Fatal("nonce unchanged — pod was not restarted by liveness probe failure")
				}
			})
		})
	}
}

func TestLifecycle_Stop(t *testing.T) {
	for _, name := range allRuntimes {
		rt, enabled := lifecycleRuntimes[name]
		t.Run(name, func(t *testing.T) {
			if !enabled {
				t.Skipf("lifecycle hooks not enabled for %s", name)
			}

			recName := fmt.Sprintf("func-e2e-lifecycle-stop-rec-%s", name)
			rec := deployRecorder(t, recName)

			funcName := fmt.Sprintf("func-e2e-test-lifecycle-stop-%s", name)
			root := fromCleanEnv(t, funcName)

			args := append([]string{"init", "-l=" + name}, rt.initArgs...)
			if err := newCmd(t, args...).Run(); err != nil {
				t.Fatal(err)
			}

			if err := newCmd(t, "config", "envs", "add",
				"--name=RECORDER_URL", "--value="+rec.internalURL).Run(); err != nil {
				t.Fatal(err)
			}

			src := lifecycleSource(t, name, "stop", rt)
			if err := os.WriteFile(filepath.Join(root, rt.srcPath), src, 0644); err != nil {
				t.Fatal(err)
			}

			deployArgs := []string{"deploy"}
			if rt.builder != "" {
				deployArgs = append(deployArgs, "--builder", rt.builder)
			}
			if err := newCmd(t, deployArgs...).Run(); err != nil {
				t.Fatal(err)
			}
			if rt.builder != "" {
				defer cleanImages(t, funcName)
			}

			if !waitFor(t, ksvcUrl(funcName), withWaitTimeout(5*time.Minute)) {
				t.Fatal("notifier function did not deploy correctly")
			}

			if err := newCmd(t, "delete", funcName, "--namespace", Namespace).Run(); err != nil {
				t.Logf("error deleting notifier: %v", err)
			}

			stopID := fmt.Sprintf("stop-%s", name)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			if !rec.awaitReceived(ctx, t, stopID) {
				t.Fatalf("recorder did not receive stop event %q within 60s", stopID)
			}
			t.Logf("Stop hook confirmed: recorder received %q", stopID)
		})
	}
}
