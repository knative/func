package function

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Function struct {
	mu          sync.Mutex
	startCalled bool
	configValue string
	ready       bool
	alive       bool
	nonce       string
}

func New() *Function {
	f := &Function{
		alive: true,
		nonce: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	go func() {
		time.Sleep(15 * time.Second)
		f.mu.Lock()
		f.ready = true
		f.mu.Unlock()
	}()
	return f
}

func (f *Function) Start(_ context.Context, cfg map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalled = true
	f.configValue = cfg["TEST_CONFIG_VALUE"]
	return nil
}

func (f *Function) Ready(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready, nil
}

func (f *Function) Alive(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive, nil
}

func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/set-unhealthy" {
		f.mu.Lock()
		f.alive = false
		f.mu.Unlock()
		fmt.Fprintln(w, "UNHEALTHY")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startCalled {
		fmt.Fprintf(w, "START_OK:%s READY=%t ALIVE=%t NONCE=%s", f.configValue, f.ready, f.alive, f.nonce)
	} else {
		fmt.Fprintf(w, "START_NOT_CALLED READY=%t ALIVE=%t NONCE=%s", f.ready, f.alive, f.nonce)
	}
}
