//go:build integration

package builders_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/buildpacks"
	fn "knative.dev/func/pkg/functions"
)

// TestInt_PackBuildCAFileRuntimes ensures that a CA given with
// f.Local.BuildCAFile is trusted by each runtime's dependency download during
// a pack build behind a TLS-intercepting proxy, the setup reported in
// https://github.com/knative/func/issues/3721.
//
// Every HTTPS connection of the build goes through a proxy which re-signs it
// with a test CA, so each runtime's package manager fetches its real
// dependencies from its public registry through it. The build passes only if
// it trusts the test CA; a Python build without the CA file is expected to
// fail.
func TestInt_PackBuildCAFileRuntimes(t *testing.T) {
	if runtime.GOOS != "linux" {
		// The pack build container reaches the proxy on 127.0.0.1 through
		// host networking, which the pack builder only uses on Linux.
		t.Skip("Skipping TestInt_PackBuildCAFileRuntimes on non-Linux systems due to host networking")
	}

	ca, caKey, caPEM := newTestCA(t, "func-test-intercepting-proxy-ca")
	caFile := filepath.Join(t.TempDir(), "proxy-ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		runtime string
		// registries are the hosts of which at least one must have been
		// fetched from through the proxy: the runtime's package registry.
		registries []string
		noCA       bool     // build without the CA file, expecting a failure
		envs       []fn.Env // extra build envs
		knownGap   string   // why the case cannot pass yet; it is skipped
	}{
		{
			// Poetry uses its own CA bundle (certifi, through requests) and
			// ignores SSL_CERT_DIR, where Paketo adds the CA; pip, which
			// installs Poetry, does honor it. REQUESTS_CA_BUNDLE replaces
			// that bundle with the proxy CA.
			name: "python", runtime: "python", registries: []string{"pypi.org", "files.pythonhosted.org"},
			envs: []fn.Env{{Name: ptr("REQUESTS_CA_BUNDLE"), Value: ptr("/platform/bindings/func-build-ca/ca-0.pem")}},
		},
		{name: "python-without-ca", runtime: "python", noCA: true},
		{name: "node", runtime: "node", registries: []string{"registry.npmjs.org"}},
		{name: "typescript", runtime: "typescript", registries: []string{"registry.npmjs.org"}},
		{
			name: "rust", runtime: "rust", registries: []string{"index.crates.io", "static.crates.io"},
			knownGap: "the builder's Rust buildpack group has no paketo-buildpacks/ca-certificates, " +
				"so the CA binding is never applied and rustup-init cannot be downloaded",
		},
		{name: "springboot", runtime: "springboot", registries: []string{"repo.maven.apache.org", "repo1.maven.org"}},
		{name: "quarkus", runtime: "quarkus", registries: []string{"repo.maven.apache.org", "repo1.maven.org"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.knownGap != "" {
				t.Skip(tt.knownGap)
			}
			// A proxy per case, so the counts only cover this build.
			proxy := serveInterceptingProxy(t, ca, caKey)
			f := newRuntimeFunction(t, tt.runtime, proxy.addr)
			f.Build.Image = fmt.Sprintf("registry.localtest.me/%s-ca:test-%d", tt.name, time.Now().UnixNano())
			f.Build.BuildEnvs = append(f.Build.BuildEnvs, tt.envs...)
			if !tt.noCA {
				f.Local.BuildCAFile = caFile
			}

			if err := buildpacks.NewScaffolder(true).Scaffold(t.Context(), f, ""); err != nil {
				t.Fatal(err)
			}
			err := buildpacks.NewBuilder(buildpacks.WithVerbose(true)).Build(t.Context(), f, nil)
			t.Logf("proxy: intercepted %v, rejected handshakes %d", proxy.interceptedHosts(), proxy.rejected.Load())

			if tt.noCA {
				if err == nil {
					t.Fatal("expected the build to fail without the CA file")
				}
				if proxy.rejected.Load() == 0 {
					t.Fatalf("expected the build to reject the proxy's certificate, build error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !proxy.interceptedAny(tt.registries...) {
				t.Fatalf("expected dependencies to be fetched through the proxy from one of %v, intercepted %v",
					tt.registries, proxy.interceptedHosts())
			}
		})
	}
}

// TestInt_PackBuildCAFileGit documents that git, which is linked against
// gnutls in the build image and reads only its own CA bundle, does not trust
// a CA given with f.Local.BuildCAFile on its own, and that GIT_SSL_CAINFO
// pointing at the mounted CA file makes it do so.
//
// A Go function fetches a module directly from its git repository
// (GOPROXY=direct) through the TLS-intercepting proxy.
func TestInt_PackBuildCAFileGit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping TestInt_PackBuildCAFileGit on non-Linux systems due to host networking")
	}

	ca, caKey, caPEM := newTestCA(t, "func-test-intercepting-proxy-ca")
	caFile := filepath.Join(t.TempDir(), "proxy-ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		gitSSLCAInfo  bool // set GIT_SSL_CAINFO to the mounted CA file
		wantErr       bool
		wantIntercept bool // git's fetch from github.com went through the proxy
	}{
		{name: "ca-file-only", wantErr: true},
		{name: "ca-file-and-git-ssl-cainfo", gitSSLCAInfo: true, wantIntercept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := serveInterceptingProxy(t, ca, caKey)
			f := newGitDepFunction(t, proxy.addr)
			f.Build.Image = fmt.Sprintf("registry.localtest.me/go-gitdep:test-%s-%d", tt.name, time.Now().UnixNano())
			f.Build.Builder = builders.Pack
			f.Local.BuildCAFile = caFile
			if tt.gitSSLCAInfo {
				// The Paketo binding holds one certificate per file; the proxy
				// CA is the only one, so it is ca-0.pem.
				f.Build.BuildEnvs = append(f.Build.BuildEnvs, fn.Env{
					Name: ptr("GIT_SSL_CAINFO"), Value: ptr("/platform/bindings/func-build-ca/ca-0.pem"),
				})
			}

			if err := buildpacks.NewScaffolder(true).Scaffold(t.Context(), f, ""); err != nil {
				t.Fatal(err)
			}
			err := buildpacks.NewBuilder(buildpacks.WithVerbose(true)).Build(t.Context(), f, nil)
			t.Logf("proxy: intercepted %v, rejected handshakes %d", proxy.interceptedHosts(), proxy.rejected.Load())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected git to reject the proxy's certificate without GIT_SSL_CAINFO")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantIntercept && !proxy.interceptedAny("github.com") {
				t.Fatalf("expected git to fetch from github.com through the proxy, intercepted %v", proxy.interceptedHosts())
			}
		})
	}
}

// newRuntimeFunction creates a function from the runtime's http template whose
// build sends its HTTPS traffic through the proxy at proxyAddr.
func newRuntimeFunction(t *testing.T, rt, proxyAddr string) fn.Function {
	t.Helper()
	root := t.TempDir()
	f, err := fn.New().Init(fn.Function{Name: rt + "-ca", Runtime: rt, Template: "http", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	f.Build.Builder = builders.Pack
	f.Build.BuildEnvs = proxyBuildEnvs(proxyAddr)
	return f
}

// newGitDepFunction writes a Go function into a new directory which imports a
// small public module fetched directly with git, and whose build sends its
// HTTPS traffic through the proxy at proxyAddr.
func newGitDepFunction(t *testing.T, proxyAddr string) fn.Function {
	t.Helper()
	const (
		dep        = "github.com/google/uuid"
		depVersion = "v1.6.0"
		// go.sum lines of dep at depVersion, as in this repository's go.sum.
		depSum = "github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=\n" +
			"github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=\n"
	)
	root := t.TempDir()
	files := map[string]string{
		"func.yaml": "specVersion: 0.36.0\nname: go-gitdep\nruntime: go\ncreated: 2025-01-01T00:00:00Z\n",
		"go.mod":    "module function\n\ngo 1.22\n\nrequire " + dep + " " + depVersion + "\n",
		"go.sum":    depSum,
		"function.go": `package function

import (
	"fmt"
	"net/http"

	"` + dep + `"
)

type Function struct{}

func New() *Function { return &Function{} }

// Handle an HTTP Request.
func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "id: %s", uuid.NewString())
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Build.BuildEnvs = append(proxyBuildEnvs(proxyAddr),
		// Fetch modules with git from their repositories rather than from
		// the module proxy, which is what a private module does.
		fn.Env{Name: ptr("GOPROXY"), Value: ptr("direct")},
		fn.Env{Name: ptr("GOSUMDB"), Value: ptr("off")},
	)
	return f
}

// proxyBuildEnvs returns build envs which send the build's HTTPS traffic
// through the proxy at proxyAddr. Package managers read HTTPS_PROXY; the JVM
// does not, so Maven gets the proxy as system properties.
func proxyBuildEnvs(proxyAddr string) []fn.Env {
	host, port, _ := net.SplitHostPort(proxyAddr)
	url := "http://" + proxyAddr
	return []fn.Env{
		{Name: ptr("HTTPS_PROXY"), Value: ptr(url)},
		{Name: ptr("https_proxy"), Value: ptr(url)},
		{Name: ptr("JAVA_TOOL_OPTIONS"), Value: ptr("-Dhttps.proxyHost=" + host + " -Dhttps.proxyPort=" + port)},
	}
}

// interceptingProxy is an HTTP proxy which intercepts HTTPS (CONNECT)
// connections: it terminates TLS with a certificate for the requested host
// signed by its CA, and relays the traffic over a new TLS connection to the
// real host, as TLS-intercepting corporate proxies do.
type interceptingProxy struct {
	addr  string
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey

	mu    sync.Mutex
	certs map[string]*tls.Certificate
	hosts map[string]int // hosts whose traffic was relayed

	// rejected counts TLS handshakes with the client that failed, as when
	// the client does not trust the proxy's CA.
	rejected atomic.Int64
}

// serveInterceptingProxy serves an intercepting proxy on 127.0.0.1 until the
// test ends.
func serveInterceptingProxy(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) *interceptingProxy {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &interceptingProxy{
		addr:  l.Addr().String(),
		ca:    ca,
		caKey: caKey,
		certs: map[string]*tls.Certificate{},
		hosts: map[string]int{},
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = l.Close()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.serve(conn)
			}()
		}
	}()
	return p
}

func (p *interceptingProxy) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		// Plain HTTP is not intercepted; the test only sends HTTPS here.
		_, _ = io.WriteString(conn, "HTTP/1.1 501 Not Implemented\r\nContent-Length: 0\r\n\r\n")
		return
	}
	host, port, err := net.SplitHostPort(req.Host)
	if err != nil {
		host, port = req.Host, "443"
	}
	if _, err = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}

	client := tls.Server(conn, &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := hello.ServerName
			if name == "" {
				name = host
			}
			return p.certFor(name)
		},
		// Relay bytes as they are, so both sides must speak HTTP/1.1.
		NextProtos: []string{"http/1.1"},
	})
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))
	if err = client.Handshake(); err != nil {
		p.rejected.Add(1)
		return
	}
	_ = client.SetDeadline(time.Time{})

	upstream, err := tls.DialWithDialer(&net.Dialer{Timeout: 30 * time.Second}, "tcp",
		net.JoinHostPort(host, port), &tls.Config{ServerName: host, NextProtos: []string{"http/1.1"}})
	if err != nil {
		return
	}
	defer upstream.Close()

	p.mu.Lock()
	p.hosts[host]++
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); _ = upstream.CloseWrite(); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); _ = client.CloseWrite(); done <- struct{}{} }()
	<-done
	<-done
}

// certFor returns a certificate for host signed by the proxy's CA.
func (p *interceptingProxy) certFor(host string) (*tls.Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.certs[host]; ok {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: randSN(),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 1),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca, &key.PublicKey, p.caKey)
	if err != nil {
		return nil, err
	}
	c := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	p.certs[host] = c
	return c, nil
}

// interceptedAny reports whether traffic to any of hosts was relayed.
func (p *interceptingProxy) interceptedAny(hosts ...string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range hosts {
		if p.hosts[h] > 0 {
			return true
		}
	}
	return false
}

// interceptedHosts returns the relayed hosts, for test logs.
func (p *interceptingProxy) interceptedHosts() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	hosts := make([]string, 0, len(p.hosts))
	for h, n := range p.hosts {
		hosts = append(hosts, fmt.Sprintf("%s(%d)", h, n))
	}
	return strings.Join(hosts, " ")
}
