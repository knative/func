package http

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"knative.dev/kn-plugin-func/k8s"
)

type RoundTripCloser interface {
	http.RoundTripper
	io.Closer
}

// NewRoundTripper returns new closable RoundTripper that first tries to dial connection in standard way,
// if the dial operation fails due to hostname resolution the RoundTripper tries to dial from in cluster pod.
//
// This is useful for accessing cluster internal services (pushing a CloudEvent into Knative broker).
func NewRoundTripper() RoundTripCloser {
	d := &dialer{
		netDialer: net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
	return &roundTripCloser{
		Transport: http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		d: d,
	}
}

type roundTripCloser struct {
	http.Transport
	d *dialer
}

func (r *roundTripCloser) Close() error {
	return r.d.Close()
}

type dialer struct {
	o               sync.Once
	netDialer       net.Dialer
	inClusterDialer k8s.ContextDialer
}

func (d *dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.netDialer.DialContext(ctx, network, address)
	if err == nil {
		return conn, nil
	}

	var dnsErr *net.DNSError
	if !(errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
		return nil, err
	}
	err = nil

	d.o.Do(func() {
		d.inClusterDialer, err = k8s.NewInClusterDialer(ctx)
	})

	if err != nil {
		return nil, err
	}

	if d.inClusterDialer == nil {
		return nil, errors.New("failed to init in cluster dialer")
	}

	return d.inClusterDialer.DialContext(ctx, network, address)
}

func (d *dialer) Close() error {
	if d.inClusterDialer != nil {
		return d.inClusterDialer.Close()
	}
	return nil
}
