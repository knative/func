package docker_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// TestNewClient_DockerContext tests that Docker context is properly detected
// when DOCKER_HOST is not set and default socket doesn't exist.
func TestNewClient_DockerContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context test on Windows")
	}

	// Check if docker CLI is available
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker CLI not available, skipping context detection test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
	defer cancel()

	tmpDir := t.TempDir()

	// Start a mock daemon on a socket in the temp directory.
	sock := filepath.Join(tmpDir, "docker.sock")
	startMockDaemonUnix(t, sock)
	sockHost := fmt.Sprintf("unix://%s", sock)

	// Build a Docker config directory with a context pointing to
	// the mock daemon socket.
	configDir := filepath.Join(tmpDir, "docker-config")
	contextName := "func-test-ctx"
	createDockerContextConfig(t, configDir, contextName, sockHost)

	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONFIG", configDir)

	// Pass a non-existent socket as the default host to force context detection.
	nonExistentDefault := fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "nonexistent.sock"))
	dockerClient, _, err := docker.NewClient(nonExistentDefault)
	if err != nil {
		if err == docker.ErrNoDocker {
			t.Skip("Docker not available, skipping context detection test")
		}
		t.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	// If we can reach the mock daemon despite passing a non-existent default
	// socket, context detection found our mock daemon's socket.
	nfo, err := dockerClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatalf("Failed to get info from mock daemon: %v", err)
	}
	if nfo.Info.ID != "mock-daemon" {
		t.Errorf("unexpected server ID: got %q, want %q", nfo.Info.ID, "mock-daemon")
	}
}

// createDockerContextConfig writes a minimal Docker CLI config directory
// with a single context pointing to the given host.
func createDockerContextConfig(t *testing.T, configDir, contextName, host string) {
	t.Helper()

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configJSON := fmt.Sprintf(`{"auths":{},"currentContext":%q}`, contextName)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Docker CLI stores context metadata under
	// contexts/meta/<sha256(contextName)>/meta.json
	hash := sha256.Sum256([]byte(contextName))
	metaDir := filepath.Join(configDir, "contexts", "meta", fmt.Sprintf("%x", hash))
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	metaJSON := fmt.Sprintf(
		`{"Name":%q,"Metadata":{"Description":"test context"},"Endpoints":{"docker":{"Host":%q,"SkipTLSVerify":false}}}`,
		contextName, host,
	)
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(metaJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startMockTLSDaemon creates a TLS-enabled mock Docker daemon for testing.
// Returns the listener, CA cert, client cert, and client key in PEM format.
func startMockTLSDaemon(t *testing.T) (net.Listener, []byte, []byte, []byte) {
	t.Helper()

	// Generate CA certificate
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Generate server certificate
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test Server"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Generate client certificate
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"Test Client"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})

	// Create TLS config for server
	// Server needs to trust the CA that signed the client cert
	clientCACertPool := x509.NewCertPool()
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatal(err)
	}
	clientCACertPool.AddCert(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{serverCertDER},
				PrivateKey:  serverKey,
			},
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  clientCACertPool,
	}

	// Start TLS listener
	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}

	// Start mock daemon with TLS
	startMockDaemon(t, listener)

	return listener, caCertPEM, clientCertPEM, clientKeyPEM
}

// TestNewClient_DockerContextTLS tests that TLS configuration from Docker context
// is properly loaded and used when connecting to remote Docker daemons.
func TestNewClient_DockerContextTLS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context TLS test on Windows")
	}

	// Check if docker CLI is available
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker CLI not available, skipping context TLS test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
	defer cancel()

	tmpDir := t.TempDir()

	// Start a mock TLS daemon
	tlsListener, caCert, clientCert, clientKey := startMockTLSDaemon(t)
	tlsHost := fmt.Sprintf("tcp://%s", tlsListener.Addr().String())

	// Build a Docker config directory with a context that has TLS configuration
	configDir := filepath.Join(tmpDir, "docker-config")
	contextName := "func-test-tls-ctx"

	// Calculate the hash for the context name (Docker uses SHA256)
	hash := sha256.Sum256([]byte(contextName))
	hashStr := fmt.Sprintf("%x", hash)

	// Docker stores TLS files in contexts/tls/<hash>/
	tlsDir := filepath.Join(configDir, "contexts", "tls", hashStr)

	// Create TLS directory and write the actual certificate files
	if err := os.MkdirAll(tlsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tlsDir, "ca.pem"), caCert, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "cert.pem"), clientCert, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "key.pem"), clientKey, 0o600); err != nil {
		t.Fatal(err)
	}

	createDockerContextConfigWithTLS(t, configDir, contextName, tlsHost, tlsDir)

	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONFIG", configDir)

	// Pass a non-existent socket as the default host to force context detection
	nonExistentDefault := fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "nonexistent.sock"))
	dockerClient, _, err := docker.NewClient(nonExistentDefault)
	if err != nil {
		t.Fatalf("Failed to create Docker client with TLS context: %v", err)
	}
	defer dockerClient.Close()

	// Verify we can connect to the TLS-enabled mock daemon
	// This proves that TLS certificates from the context are actually being used
	_, err = dockerClient.Ping(ctx, client.PingOptions{})
	if err != nil {
		t.Fatalf("Failed to ping TLS-enabled mock daemon: %v", err)
	}

	// Verify we're actually talking to our mock daemon
	nfo, err := dockerClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatalf("Failed to get info from TLS mock daemon: %v", err)
	}
	if nfo.Info.ID != "mock-daemon" {
		t.Errorf("unexpected server ID: got %q, want %q", nfo.Info.ID, "mock-daemon")
	}
}

// createDockerContextConfigWithTLS writes a Docker CLI config directory
// with a context that includes TLS configuration.
func createDockerContextConfigWithTLS(t *testing.T, configDir, contextName, host, tlsPath string) {
	t.Helper()

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configJSON := fmt.Sprintf(`{"auths":{},"currentContext":%q}`, contextName)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256([]byte(contextName))
	metaDir := filepath.Join(configDir, "contexts", "meta", fmt.Sprintf("%x", hash))
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	metaJSON := fmt.Sprintf(
		`{"Name":%q,"Metadata":{"Description":"test context with TLS"},"Endpoints":{"docker":{"Host":%q,"SkipTLSVerify":false}},"Storage":{"MetadataPath":%q,"TLSPath":%q}}`,
		contextName, host, metaDir, tlsPath,
	)
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(metaJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNewClient_DockerContextTLS_FallbackPath tests that TLS configuration is properly
// loaded even when storage.TLSPath is "<IN MEMORY>" or empty, by falling back to the
// calculated path based on the context name hash.
func TestNewClient_DockerContextTLS_FallbackPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Docker context TLS fallback test on Windows")
	}

	// Check if docker CLI is available
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker CLI not available, skipping context TLS fallback test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
	defer cancel()

	tmpDir := t.TempDir()

	// Start a mock TLS daemon
	tlsListener, caCert, clientCert, clientKey := startMockTLSDaemon(t)
	tlsHost := fmt.Sprintf("tcp://%s", tlsListener.Addr().String())

	// Build a Docker config directory with a context that has TLS configuration
	configDir := filepath.Join(tmpDir, "docker-config")
	contextName := "func-test-tls-fallback-ctx"

	// Calculate the hash for the context name (Docker uses SHA256)
	hash := sha256.Sum256([]byte(contextName))
	hashStr := fmt.Sprintf("%x", hash)

	// Docker stores TLS files in contexts/tls/<hash>/
	tlsDir := filepath.Join(configDir, "contexts", "tls", hashStr)

	// Create TLS directory and write the actual certificate files
	if err := os.MkdirAll(tlsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tlsDir, "ca.pem"), caCert, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "cert.pem"), clientCert, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "key.pem"), clientKey, 0o600); err != nil {
		t.Fatal(err)
	}

	// Create context config with TLSPath set to "<IN MEMORY>" to test fallback
	createDockerContextConfigWithTLS(t, configDir, contextName, tlsHost, "<IN MEMORY>")

	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONFIG", configDir)

	// Pass a non-existent socket as the default host to force context detection
	nonExistentDefault := fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "nonexistent.sock"))
	dockerClient, _, err := docker.NewClient(nonExistentDefault)
	if err != nil {
		t.Fatalf("Failed to create Docker client with TLS context (fallback path): %v", err)
	}
	defer dockerClient.Close()

	// Verify we can connect to the TLS-enabled mock daemon
	// This proves that TLS certificates were found via the fallback path calculation
	_, err = dockerClient.Ping(ctx, client.PingOptions{})
	if err != nil {
		t.Fatalf("Failed to ping TLS-enabled mock daemon (fallback path): %v", err)
	}

	// Verify we're actually talking to our mock daemon
	nfo, err := dockerClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatalf("Failed to get info from TLS mock daemon (fallback path): %v", err)
	}
	if nfo.Info.ID != "mock-daemon" {
		t.Errorf("unexpected server ID: got %q, want %q", nfo.Info.ID, "mock-daemon")
	}
}
