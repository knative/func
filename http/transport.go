package http

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
	
	"knative.dev/kn-plugin-func/k8s"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
	Close() error
}

type RoundTripCloser interface {
	http.RoundTripper
	io.Closer
}

// NewRoundTripper returns new closable RoundTripper that first tries to dial connection in standard way,
// if the dial operation fails due to hostname resolution the RoundTripper tries to dial from in cluster pod.
//
// This is useful for accessing cluster internal services (pushing a CloudEvent into Knative broker).
func NewRoundTripper() RoundTripCloser {
	httpTransport := newHTTPTransport()

	primaryDialer := dialContextFn(httpTransport.DialContext)
	secondaryDialer := k8s.NewLazyInitInClusterDialer()
	combinedDialer := newDialerWithFallback(primaryDialer, secondaryDialer)

	httpTransport.DialContext = combinedDialer.DialContext
	httpTransport.DialTLSContext = nil

	return &roundTripCloser{
		Transport:       httpTransport,
		primaryDialer:   primaryDialer,
		secondaryDialer: secondaryDialer,
	}
}

func newHTTPTransport() *http.Transport {
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return dt.Clone()
	} else {
		return &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}
}

type roundTripCloser struct {
	*http.Transport
	primaryDialer   ContextDialer
	secondaryDialer ContextDialer
}

func (r *roundTripCloser) Close() error {
	err := r.primaryDialer.Close()
	if err != nil {
		return err
	}
	return r.secondaryDialer.Close()
}

func newDialerWithFallback(primaryDialer ContextDialer, fallbackDialer ContextDialer) *dialerWithFallback {
	return &dialerWithFallback{
		primaryDialer:  primaryDialer,
		fallbackDialer: fallbackDialer,
	}
}

type dialerWithFallback struct {
	primaryDialer  ContextDialer
	fallbackDialer ContextDialer
}

func (d *dialerWithFallback) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.primaryDialer.DialContext(ctx, network, address)
	if err == nil {
		return conn, nil
	}

	var dnsErr *net.DNSError
	if !(errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
		return nil, err
	}

	return d.fallbackDialer.DialContext(ctx, network, address)
}

func (d *dialerWithFallback) Close() error {
	d.primaryDialer.Close()
	d.fallbackDialer.Close()
	return nil
}

type dialContextFn func(ctx context.Context, network string, addr string) (net.Conn, error)

func (d dialContextFn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	return d(ctx, network, addr)
}

func (d dialContextFn) Close() error { return nil }
