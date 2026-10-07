package shared

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// HTTPClient is used for all outbound HTTP requests; has a 30-second timeout.
var HTTPClient = &http.Client{Timeout: 30 * time.Second}

// NotifyContext returns a copy of parent that is canceled on SIGINT or SIGTERM.
// Callers should defer the returned stop function.
func NotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}
