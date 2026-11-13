package docker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	ctxdocker "github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"

	"knative.dev/func/pkg/docker"
)

// Test that we are creating client in accordance
// with the DOCKER_HOST environment variable
func TestNewClient(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TODO fix this test on Windows CI") // TODO fix this
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute*1)
	defer cancel()

	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "docker.sock")
	dockerHost := fmt.Sprintf("unix://%s", sock)

	startMockDaemonUnix(t, sock)

	t.Setenv("DOCKER_HOST", dockerHost)

	dockerClient, dockerHostInRemote, err := docker.NewClient(client.DefaultDockerHost)
	if err != nil {
		t.Error(err)
	}
	defer dockerClient.Close()

	if runtime.GOOS == "linux" && dockerHostInRemote != dockerHost {
		t.Errorf("unexpected dockerHostInRemote: expected %q, but got %q", dockerHost, dockerHostInRemote)
	}
	if runtime.GOOS == "darwin" && dockerHostInRemote != "" {
		t.Errorf("unexpected dockerHostInRemote: expected empty string, but got %q", dockerHostInRemote)
	}

	_, err = dockerClient.Ping(ctx, client.PingOptions{})
	if err != nil {
		t.Error(err)
	}
}

func TestNewClient_DockerHost(t *testing.T) {
	tests := []struct {
		name                     string
		dockerHostEnvVar         string
		expectedRemoteDockerHost map[string]string
	}{
		{
			name:                     "tcp",
			dockerHostEnvVar:         "tcp://10.0.0.1:1234",
			expectedRemoteDockerHost: map[string]string{"darwin": "", "windows": "", "linux": ""},
		},
		{
			name:                     "unix",
			dockerHostEnvVar:         "unix:///some/path/docker.sock",
			expectedRemoteDockerHost: map[string]string{"darwin": "", "windows": "", "linux": "unix:///some/path/docker.sock"},
		},
		{
			name:                     "Docker Desktop",
			dockerHostEnvVar:         "unix:///home/jdoe/.docker/desktop/docker.sock",
			expectedRemoteDockerHost: map[string]string{"darwin": "", "windows": "", "linux": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.HasPrefix(tt.dockerHostEnvVar, "unix") && runtime.GOOS == "windows" {
				t.Skip("Windows cannot handle Unix sockets")
			}

			t.Setenv("DOCKER_HOST", tt.dockerHostEnvVar)
			_, host, err := docker.NewClient(client.DefaultDockerHost)
			if err != nil {
				t.Fatal(err)
			}
			expectedRemoteDockerHost := tt.expectedRemoteDockerHost[runtime.GOOS]
			if host != expectedRemoteDockerHost {
				t.Errorf("expected docker host %q, but got %q", expectedRemoteDockerHost, host)
			}
		})
	}

}

func startMockDaemon(t *testing.T, listener net.Listener) {
	mux := http.NewServeMux()

	// mimics /_ping endpoint
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// mimics /info endpoint (also matched as /v{version}/info)
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		info := system.Info{
			ID:            "mock-daemon",
			ServerVersion: "0.0.0-mock",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	server := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Docker client prefixes paths with /v{version},
			// e.g. /v1.47/_ping or /v1.47/info.
			// Strip the version prefix so the mux can match.
			p := r.URL.Path
			if strings.HasPrefix(p, "/v") {
				if i := strings.Index(p[1:], "/"); i != -1 {
					r.URL.Path = p[1+i:]
				}
			}
			mux.ServeHTTP(w, r)
		}),
	}

	serErrChan := make(chan error)
	go func() {
		serErrChan <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		err := server.Shutdown(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		err = <-serErrChan
		if err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Fatal(err)
		}
	})
}

func startMockDaemonUnix(t *testing.T, sock string) {
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	startMockDaemon(t, l)
}

// TestNewClient_DockerContext tests that the unix socket of the current Docker
// context is used when DOCKER_HOST is not set, whether or not the default
// host exists.
func TestNewClient_DockerContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}

	for _, existingDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing default host %v", existingDefault), func(t *testing.T) {
			unsetEnv(t, "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH")

			tmpDir := t.TempDir()

			// Start a mock daemon on a socket in a short directory. t.TempDir()
			// includes the subtest name, which on macOS makes the path longer
			// than the 104 byte limit for unix socket paths.
			sock := filepath.Join(shortTempDir(t), "docker.sock")
			startMockDaemonUnix(t, sock)

			configDir := filepath.Join(tmpDir, "docker-config")
			createDockerContext(t, configDir, "func-test-ctx", "unix://"+sock, nil)
			t.Setenv("DOCKER_CONFIG", configDir)

			assertMockDaemonInfo(t, defaultHost(t, tmpDir, existingDefault), false)
		})
	}
}

// TestNewClient_DockerContextTLS tests that the TLS material stored in the
// current Docker context is used for TCP connections.
func TestNewClient_DockerContextTLS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}

	ca := newTestCA(t)
	otherCA := newTestCA(t)

	tests := []struct {
		name              string
		requireClientCert bool
		ctxTLS            map[string][]byte
		existingDefault   bool
		wantInfoErr       bool
	}{
		{
			name:              "mTLS, non-existent default host",
			requireClientCert: true,
			ctxTLS:            ca.clientFiles(t),
		},
		{
			name:              "mTLS, existing default host",
			requireClientCert: true,
			ctxTLS:            ca.clientFiles(t),
			existingDefault:   true,
		},
		{
			name:   "CA only",
			ctxTLS: map[string][]byte{"ca.pem": ca.certPEM},
		},
		{
			name:              "CA only, server requires client cert",
			requireClientCert: true,
			ctxTLS:            map[string][]byte{"ca.pem": ca.certPEM},
			wantInfoErr:       true,
		},
		{
			name:        "no TLS in context",
			wantInfoErr: true,
		},
		{
			name:        "wrong CA",
			ctxTLS:      map[string][]byte{"ca.pem": otherCA.certPEM},
			wantInfoErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH")

			tmpDir := t.TempDir()
			host := startMockDaemonTLS(t, ca, tt.requireClientCert)

			configDir := filepath.Join(tmpDir, "docker-config")
			createDockerContext(t, configDir, "func-tls-ctx", host, tt.ctxTLS)
			t.Setenv("DOCKER_CONFIG", configDir)

			if tt.ctxTLS != nil {
				// Docker stores endpoint TLS files under contexts/tls/<sha256(name)>/<endpoint>/.
				caFile := filepath.Join(configDir, "contexts", "tls", sha256hex("func-tls-ctx"), "docker", "ca.pem")
				if _, err := os.Stat(caFile); err != nil {
					t.Fatalf("expected context CA at docker's location: %v", err)
				}
			}

			assertMockDaemonInfo(t, defaultHost(t, tmpDir, tt.existingDefault), tt.wantInfoErr)
		})
	}
}

// TestNewClient_TLSEnvOverridesContext tests that DOCKER_TLS_VERIFY and
// DOCKER_CERT_PATH take precedence over the TLS material of the context.
func TestNewClient_TLSEnvOverridesContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}
	unsetEnv(t, "DOCKER_HOST", "DOCKER_CONTEXT")

	ca := newTestCA(t)
	otherCA := newTestCA(t)
	tmpDir := t.TempDir()
	host := startMockDaemonTLS(t, ca, true)

	configDir := filepath.Join(tmpDir, "docker-config")
	createDockerContext(t, configDir, "func-tls-ctx", host, otherCA.clientFiles(t))
	t.Setenv("DOCKER_CONFIG", configDir)

	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", writeCertDir(t, ca.clientFiles(t)))

	assertMockDaemonInfo(t, defaultHost(t, tmpDir, false), false)
}

// TestNewClient_DockerHostIgnoresContextTLS tests that the context TLS
// material is not used when the host comes from DOCKER_HOST.
func TestNewClient_DockerHostIgnoresContextTLS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}
	unsetEnv(t, "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH")

	ca := newTestCA(t)
	tmpDir := t.TempDir()
	host := startMockDaemonTLS(t, ca, true)

	configDir := filepath.Join(tmpDir, "docker-config")
	createDockerContext(t, configDir, "func-tls-ctx", host, ca.clientFiles(t))
	t.Setenv("DOCKER_CONFIG", configDir)
	t.Setenv("DOCKER_HOST", host)

	assertMockDaemonInfo(t, defaultHost(t, tmpDir, false), true)
}

// TestNewClient_TLSEnv tests TLS configured only by environment variables.
func TestNewClient_TLSEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping TLS test on Windows")
	}
	unsetEnv(t, "DOCKER_CONTEXT")

	ca := newTestCA(t)
	tmpDir := t.TempDir()
	host := startMockDaemonTLS(t, ca, true)

	t.Setenv("DOCKER_CONFIG", filepath.Join(tmpDir, "docker-config"))
	t.Setenv("DOCKER_HOST", host)
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", writeCertDir(t, ca.clientFiles(t)))

	assertMockDaemonInfo(t, defaultHost(t, tmpDir, false), false)
}

// TestNewClient_DockerContextInvalidKey tests that a broken context key is
// reported instead of silently connecting without TLS.
func TestNewClient_DockerContextInvalidKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}
	unsetEnv(t, "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH")

	ca := newTestCA(t)
	tmpDir := t.TempDir()
	files := ca.clientFiles(t)
	files["key.pem"] = []byte("not a key")

	configDir := filepath.Join(tmpDir, "docker-config")
	createDockerContext(t, configDir, "func-bad-key", "tcp://127.0.0.1:1", files)
	t.Setenv("DOCKER_CONFIG", configDir)

	dc, _, err := docker.NewClient(defaultHost(t, tmpDir, false))
	if err == nil {
		dc.Close()
		t.Fatal("expected an error for malformed key.pem")
	}
	if !strings.Contains(err.Error(), "func-bad-key") {
		t.Errorf("error should mention the context name: %v", err)
	}
}

// assertMockDaemonInfo creates a client and checks that Info reaches the mock
// daemon (or fails when wantErr is set).
func assertMockDaemonInfo(t *testing.T, defaultHost string, wantErr bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*10)
	defer cancel()

	dockerClient, _, err := docker.NewClient(defaultHost)
	if err != nil {
		t.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	nfo, err := dockerClient.Info(ctx, client.InfoOptions{})
	if wantErr {
		if err == nil {
			t.Fatal("expected Info to fail")
		}
		return
	}
	if err != nil {
		t.Fatalf("Failed to get info from mock daemon: %v", err)
	}
	if nfo.Info.ID != "mock-daemon" {
		t.Errorf("unexpected server ID: got %q, want %q", nfo.Info.ID, "mock-daemon")
	}
}

// defaultHost returns a unix default host in dir, either a non-existent path
// or an existing empty file.
func defaultHost(t *testing.T, dir string, existing bool) string {
	t.Helper()
	p := filepath.Join(dir, "default.sock")
	if existing {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return "unix://" + p
}

// shortTempDir returns a temporary directory with a short path, suitable for
// unix sockets. On macOS $TMPDIR is already long, so /tmp is used there.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "fdc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// unsetEnv unsets the given environment variables for the duration of the test.
// Unlike t.Setenv(key, ""), the variables are really absent for os.LookupEnv.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { _ = os.Setenv(key, v) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(key) })
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func sha256hex(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

// createDockerContext creates a Docker context and makes it current, using the
// docker/cli context store so the on-disk layout is exactly docker's.
func createDockerContext(t *testing.T, configDir, name, host string, tlsFiles map[string][]byte) {
	t.Helper()

	s := store.New(filepath.Join(configDir, "contexts"), store.NewConfig(nil,
		store.EndpointTypeGetter(ctxdocker.DockerEndpoint, func() any { return &ctxdocker.EndpointMeta{} })))
	err := s.CreateOrUpdate(store.Metadata{
		Name: name,
		Endpoints: map[string]any{
			ctxdocker.DockerEndpoint: ctxdocker.EndpointMeta{Host: host},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tlsFiles != nil {
		err = s.ResetEndpointTLSMaterial(name, ctxdocker.DockerEndpoint, &store.EndpointTLSData{Files: tlsFiles})
		if err != nil {
			t.Fatal(err)
		}
	}

	configJSON := fmt.Sprintf(`{"auths":{},"currentContext":%q}`, name)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeCertDir writes files to a new directory usable as DOCKER_CERT_PATH.
func writeCertDir(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type testCA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "func test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// issue returns PEM encoded certificate and key signed by the CA.
func (ca *testCA) issue(t *testing.T, server bool) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "func test client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if server {
		tmpl.Subject.CommonName = "func test server"
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// clientFiles returns ca.pem, cert.pem and key.pem for a client of the CA.
func (ca *testCA) clientFiles(t *testing.T) map[string][]byte {
	t.Helper()
	certPEM, keyPEM := ca.issue(t, false)
	return map[string][]byte{"ca.pem": ca.certPEM, "cert.pem": certPEM, "key.pem": keyPEM}
}

// startMockDaemonTLS starts a mock daemon serving TLS with a certificate
// signed by ca and returns its tcp:// host.
func startMockDaemonTLS(t *testing.T, ca *testCA, requireClientCert bool) string {
	t.Helper()
	certPEM, keyPEM := ca.issue(t, true)
	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)

	cfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	if requireClientCert {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	startMockDaemon(t, tls.NewListener(l, cfg))
	return "tcp://" + l.Addr().String()
}
