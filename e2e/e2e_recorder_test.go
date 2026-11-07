//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
)

// The recorder is a tiny HTTP Function deployed into the cluster that
// serves as an observation point for E2E tests exercising event delivery.
// Subscribers under test POST to /record?id=X on event receipt; the test
// runner polls /received?id=X for an HTTP 200. This is a dogfooded
// replacement for scraping subscriber stdout via kubectl logs: the
// system-under-test is what's being used to validate itself.

// recorderSource is a minimal in-memory event recorder served as a
// function. One-replica assumption is acceptable because each test
// deploys its own recorder and drives low request volume.
const recorderSource = `package function

import (
	"net/http"
	"sync"
)

type Function struct {
	mu       sync.Mutex
	received map[string]bool
}

func New() *Function {
	return &Function{received: make(map[string]bool)}
}

func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	switch r.URL.Path {
	case "/record":
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.received[id] = true
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case "/received":
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		ok := f.received[id]
		f.mu.Unlock()
		if ok {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		// Readiness / health probe. waitFor polls the root and expects OK.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
`

// subscriberSource is the target of a test Subscription — a CloudEvents
// handler that forwards each event's id to the recorder at
// $RECORDER_URL/record?id=<id> so the test runner can observe delivery.
//
// Returning nil yields an empty response body; Knative broker-filter
// rejects a dispatch with "received a non-empty response not recognized
// as CloudEvent" if the subscriber writes anything else.
const subscriberSource = `package function

import (
	"net/http"
	"os"

	"github.com/cloudevents/sdk-go/v2/event"
)

type Function struct {
	recorderURL string
}

func New() *Function {
	return &Function{recorderURL: os.Getenv("RECORDER_URL")}
}

func (f *Function) Handle(e event.Event) error {
	if id := e.ID(); id != "" && f.recorderURL != "" {
		if resp, err := http.Post(f.recorderURL+"/record?id="+id, "text/plain", nil); err == nil {
			resp.Body.Close()
		}
	}
	return nil
}
`

// recorder is a handle to a deployed recorder function.
type recorder struct {
	name        string
	route       string // described route; the test runner reaches it via httpClient
	internalURL string // reachable from in-cluster workloads; pass as RECORDER_URL
}

// deployRecorder stands up a recorder function with the default deployer
// and blocks until it's ready. t.Cleanup is registered to delete the
// function at test end.
func deployRecorder(t *testing.T, name string) *recorder {
	t.Helper()

	root := fromCleanEnv(t, name)
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "function.go"),
		[]byte(recorderSource), 0644); err != nil {
		t.Fatal(err)
	}

	// Keep the recorder scaled to at least 1 replica to avoid cold-start latency
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	minScale := int64(1)
	f.Scale = &fn.ScaleOptions{Min: &minScale}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clean(t, name, Namespace) })

	route := functionURL(t, name)
	if !waitFor(t, route) {
		t.Fatal("recorder did not become ready")
	}

	return &recorder{
		name:  name,
		route: route,
		// Every deployer gives the function a Kubernetes Service of its own
		// name in the same namespace. The subscriber uses this to reach the
		// recorder without going back out through an external ingress.
		internalURL: fmt.Sprintf("http://%s.%s.svc.cluster.local", name, Namespace),
	}
}

// awaitReceived polls the recorder's route until it reports that
// the given event id has been recorded, or the context is canceled.
// Returns true on observed receipt.
func (r *recorder) awaitReceived(ctx context.Context, t *testing.T, id string) bool {
	t.Helper()
	url := r.route + "/received?id=" + id
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			t.Fatalf("building recorder request: %v", err)
		}
		resp, err := httpClient(t).Do(req)
		if err == nil {
			code := resp.StatusCode
			resp.Body.Close()
			if code == http.StatusOK {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
