package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"knative.dev/func/pkg/k8s"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
	Close() error
}

type RoundTripCloser interface {
	http.RoundTripper
	io.Closer
}

type options struct {
	selectCA           func(ctx context.Context, serverName string) (*x509.Certificate, error)
	inClusterDialer    ContextDialer
	insecureSkipVerify bool
}

type Option func(*options)

func WithSelectCA(selectCA func(ctx context.Context, serverName string) (*x509.Certificate, error)) Option {
	return func(o *options) {
		o.selectCA = selectCA
	}
}

func WithInClusterDialer(inClusterDialer ContextDialer) Option {
	return func(o *options) {
		o.inClusterDialer = inClusterDialer
	}
}

func WithInsecureSkipVerify(insecureSkipVerify bool) Option {
	return func(o *options) {
		o.insecureSkipVerify = insecureSkipVerify
	}
}

// NewRoundTripper returns new closable RoundTripper that first tries to dial connection in standard way,
// if the dial operation fails due to hostname resolution the RoundTripper tries to dial from in cluster pod.
//
// This is useful for accessing cluster internal services (pushing a CloudEvent into Knative broker).
func NewRoundTripper(opts ...Option) RoundTripCloser {
	o := options{
		inClusterDialer:    k8s.NewLazyInitInClusterDialer(),
		insecureSkipVerify: false,
	}
	for _, option := range opts {
		option(&o)
	}

	httpTransport := newHTTPTransport()

	primaryDialer := dialContextFn(httpTransport.DialContext)
	secondaryDialer := o.inClusterDialer

	combinedDialer := newDialerWithFallback(primaryDialer, secondaryDialer)

	httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: o.insecureSkipVerify}

	httpTransport.DialContext = combinedDialer.DialContext

	httpTransport.DialTLSContext = newDialTLSContext(combinedDialer, httpTransport.TLSClientConfig, o.selectCA)

	return &roundTripCloser{
		Transport: httpTransport,
		dialer:    combinedDialer,
	}
}

func newHTTPTransport() *http.Transport {
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return dt.Clone()
	} else {
		return &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Minute,
				KeepAlive: time.Minute,
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
	dialer ContextDialer
}

func (r *roundTripCloser) Close() error {
	return r.dialer.Close()
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
	if !errors.As(err, &dnsErr) {
		return nil, err
	}

	return d.fallbackDialer.DialContext(ctx, network, address)
}

func (d *dialerWithFallback) Close() error {
	var err error
	errs := make([]error, 0, 2)

	err = d.primaryDialer.Close()
	if err != nil {
		errs = append(errs, err)
	}

	err = d.fallbackDialer.Close()
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to Close(): %v", errs)
	}

	return nil
}

type dialContextFn func(ctx context.Context, network string, addr string) (net.Conn, error)

func (d dialContextFn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	return d(ctx, network, addr)
}

func (d dialContextFn) Close() error { return nil }

func newDialTLSContext(dialer ContextDialer, config *tls.Config, selectCA func(ctx context.Context, serverName string) (*x509.Certificate, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if selectCA == nil {
		return nil
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {

		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		var cfg *tls.Config
		if config != nil {
			cfg = config.Clone()
		} else {
			cfg = &tls.Config{}
		}

		serverName, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		if cfg.ServerName == "" {
			cfg.ServerName = serverName
		}

		if ca, err := selectCA(ctx, serverName); ca != nil && err == nil {
			caPool := x509.NewCertPool()
			caPool.AddCert(ca)
			cfg.RootCAs = caPool
		}

		tlsConn := tls.Client(conn, cfg)
		return tlsConn, nil
	}
}
