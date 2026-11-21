//go:build integration

package builders_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
	"golang.org/x/term"

	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/jsonmessage"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/s2i"
)

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode())
	})
}

func TestInt_PrivateGitRepository(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping TestPrivateGitRepository on non-Linux systems due to cluster networking limitations")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs // second sigint/sigterm is treated as sigkill
		os.Exit(137)
	}()

	certDir := createCertificate(t)
	t.Log("certDir:", certDir)

	servePrivateGit(ctx, t, certDir)
	t.Log("git server initiated")

	select {
	case <-time.After(time.Second * 5):
		break
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	gitCredsDir := t.TempDir()
	err := os.WriteFile(filepath.Join(gitCredsDir, "type"), []byte(`git-credentials`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	gitCred := `url=https://git-private.localtest.me
username=developer
password=nbusr123
`
	err = os.WriteFile(filepath.Join(gitCredsDir, "credentials"), []byte(gitCred), 0644)
	if err != nil {
		t.Fatal(err)
	}

	netrc := filepath.Join(t.TempDir(), ".netrc")
	netrcContent := `machine git-private.localtest.me login developer password nbusr123`
	err = os.WriteFile(netrc, []byte(netrcContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()

	src := filepath.Join("testdata", "go-fn-with-private-deps")
	dst := filepath.Join(tmpDir, "go-fn-with-private-deps")

	err = copyDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(dst)
	if err != nil {
		t.Fatal(err)
	}
	f.Build.BuildEnvs = []fn.Env{
		{
			Name:  ptr("GOPRIVATE"),
			Value: ptr("*.localtest.me"),
		},
	}

	testCases := []struct {
		Name         string
		Scaffolder   fn.Scaffolder
		Builder      fn.Builder
		BuilderImage func(ctx context.Context, t *testing.T, certDir string) string
		Envs         []fn.Env
		Mounts       []fn.MountSpec
	}{
		{
			Name:         "s2i",
			Scaffolder:   s2i.NewScaffolder(true),
			Builder:      s2i.NewBuilder(s2i.WithVerbose(true)),
			BuilderImage: buildPatchedS2IBuilder,
			Mounts: []fn.MountSpec{
				{
					Source:      netrc,
					Destination: "/opt/app-root/src/.netrc",
				}},
		},
		{
			Name:       "pack",
			Scaffolder: buildpacks.NewScaffolder(true),
			Builder:    buildpacks.NewBuilder(buildpacks.WithVerbose(true)),
			// The private module is fetched by git, which in jammy is linked
			// against gnutls and ignores SSL_CERT_DIR, where the Paketo CA
			// Certificates buildpack adds a CA given with f.Build.CAFile.
			// So this case still needs a builder image that trusts our CA.
			// See TestInt_PackBuildTrustsCAFile for f.Build.CAFile.
			BuilderImage: buildPatchedBuildpackBuilder,
			Envs: []fn.Env{
				{
					Name:  ptr("SERVICE_BINDING_ROOT"),
					Value: ptr("/bindings"),
				},
			},
			Mounts: []fn.MountSpec{
				{
					Source:      gitCredsDir,
					Destination: "/bindings/git-binding",
				},
			},
		},
	}

	for _, tt := range testCases {
		var f = f
		t.Run(tt.Name, func(t *testing.T) {
			f.Build.Image = "registry.localtest.me/go-app:test-" + tt.Name
			f.Build.Builder = tt.Name
			f.Build.BuilderImages = map[string]string{
				tt.Name: tt.BuilderImage(ctx, t, certDir),
			}
			f.Build.Mounts = append(f.Build.Mounts, tt.Mounts...)
			f.Build.BuildEnvs = append(f.Build.BuildEnvs, tt.Envs...)

			err = tt.Scaffolder.Scaffold(ctx, f, "")
			if err != nil {
				t.Fatal(err)
			}
			err = tt.Builder.Build(ctx, f, nil)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestInt_PackBuildTrustsCAFile ensures that a CA given with f.Build.CAFile is
// trusted during a pack build with a stock builder image. The function
// depends on a module served by a Go module proxy over TLS with a certificate
// signed by a custom CA, as with a TLS-intercepting proxy. The same build
// fails without the CA file.
func TestInt_PackBuildTrustsCAFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		// The pack build container reaches the proxy on 127.0.0.1 through
		// host networking, which the pack builder only uses on Linux.
		t.Skip("Skipping TestInt_PackBuildTrustsCAFile on non-Linux systems due to host networking")
	}
	const stockBuilder = "ghcr.io/knative/builder-jammy-tiny:v2"

	caCert, caKey, caPEM := newTestCA(t, "func-test-ca")
	_, _, otherPEM := newTestCA(t, "func-test-unrelated-ca")
	proxy := serveGoModuleProxy(t, caCert, caKey)

	caFile := filepath.Join(t.TempDir(), "ca.pem") // outside the function tree
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		caFile  func(root string) string // returns f.Build.CAFile, may write into root
		wantErr bool
	}{
		{
			name:   "with-ca",
			caFile: func(string) string { return caFile },
		},
		{
			name:    "without-ca",
			caFile:  func(string) string { return "" },
			wantErr: true,
		},
		{
			// A bundle with an unrelated CA first, as corporate bundles hold
			// many CAs, given relative to the function root.
			name: "bundle",
			caFile: func(root string) string {
				bundle := append(append([]byte{}, otherPEM...), caPEM...)
				if err := os.WriteFile(filepath.Join(root, "bundle.pem"), bundle, 0644); err != nil {
					t.Fatal(err)
				}
				return "bundle.pem"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			f := newCADepFunction(t, proxy)
			// Unique per run so that no module cache of an earlier build is reused.
			f.Build.Image = fmt.Sprintf("registry.localtest.me/go-cadep:test-%s-%d", tt.name, time.Now().UnixNano())
			f.Build.Builder = builders.Pack
			f.Build.BuilderImages = map[string]string{builders.Pack: stockBuilder}
			f.Build.CAFile = tt.caFile(f.Root)

			zips, handshakeErrors := proxy.zips.Load(), proxy.handshakeErrors.Load()
			if err := buildpacks.NewScaffolder(true).Scaffold(ctx, f, ""); err != nil {
				t.Fatal(err)
			}
			err := buildpacks.NewBuilder(buildpacks.WithVerbose(true)).Build(ctx, f, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected the build to fail without the CA file")
				}
				// The proxy sees the build reject its certificate.
				if proxy.handshakeErrors.Load() == handshakeErrors {
					t.Fatalf("expected a TLS handshake error at the module proxy, build error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if proxy.zips.Load() == zips {
				t.Fatal("expected the module to be downloaded from the TLS module proxy")
			}
		})
	}
}

const (
	caDepModule  = "example.localtest.me/cadep"
	caDepVersion = "v1.0.0"
)

// goModuleProxy is a Go module proxy serving caDepModule over TLS.
type goModuleProxy struct {
	url   string
	goSum string // go.sum lines of caDepModule
	zips  atomic.Int64
	// handshakeErrors counts TLS handshakes rejected by clients.
	handshakeErrors atomic.Int64
}

// serveGoModuleProxy serves a Go module proxy with caDepModule on
// https://127.0.0.1:<random port>, with a certificate signed by the given CA.
// Unknown paths return 404, which makes go try the next proxy in GOPROXY.
func serveGoModuleProxy(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) *goModuleProxy {
	t.Helper()
	m := module.Version{Path: caDepModule, Version: caDepVersion}
	modData := []byte("module " + caDepModule + "\n\ngo 1.22\n")

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), modData, 0644); err != nil {
		t.Fatal(err)
	}
	code := "package cadep\n\n// Answer returns the answer.\nfunc Answer() int { return 42 }\n"
	if err := os.WriteFile(filepath.Join(src, "cadep.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	zipFile := filepath.Join(t.TempDir(), "cadep.zip")
	zf, err := os.Create(zipFile)
	if err != nil {
		t.Fatal(err)
	}
	if err = modzip.CreateFromDir(zf, m, src); err != nil {
		t.Fatal(err)
	}
	if err = zf.Close(); err != nil {
		t.Fatal(err)
	}
	zipData, err := os.ReadFile(zipFile)
	if err != nil {
		t.Fatal(err)
	}
	zipHash, err := dirhash.HashZip(zipFile, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	modHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(modData)), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	p := &goModuleProxy{
		goSum: fmt.Sprintf("%s %s %s\n%s %s/go.mod %s\n", caDepModule, caDepVersion, zipHash, caDepModule, caDepVersion, modHash),
	}
	escaped, err := module.EscapePath(caDepModule)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/" + escaped + "/@v/"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case prefix + "list":
			fmt.Fprintln(w, caDepVersion)
		case prefix + caDepVersion + ".info":
			fmt.Fprintf(w, `{"Version":%q,"Time":"2025-01-01T00:00:00Z"}`, caDepVersion)
		case prefix + caDepVersion + ".mod":
			_, _ = w.Write(modData)
		case prefix + caDepVersion + ".zip":
			p.zips.Add(1)
			_, _ = w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{newServerCert(t, ca, caKey)}}
	srv.Config.ErrorLog = log.New(handshakeErrorCounter{&p.handshakeErrors}, "", 0)
	srv.StartTLS() // listens on 127.0.0.1
	t.Cleanup(srv.Close)
	p.url = srv.URL
	return p
}

// handshakeErrorCounter counts TLS handshake errors logged by http.Server.
type handshakeErrorCounter struct{ n *atomic.Int64 }

func (c handshakeErrorCounter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("TLS handshake error")) {
		c.n.Add(1)
	}
	return len(p), nil
}

// newCADepFunction writes a Go function into a new directory which imports
// caDepModule, with a complete go.sum, and build envs which fetch modules
// from the given proxy first.
func newCADepFunction(t *testing.T, proxy *goModuleProxy) fn.Function {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"func.yaml": "specVersion: 0.36.0\nname: go-cadep\nruntime: go\ncreated: 2025-01-01T00:00:00Z\n",
		"go.mod":    "module function\n\ngo 1.22\n\nrequire " + caDepModule + " " + caDepVersion + "\n",
		"go.sum":    proxy.goSum,
		"function.go": `package function

import (
	"fmt"
	"net/http"

	"` + caDepModule + `"
)

type Function struct{}

func New() *Function { return &Function{} }

// Handle an HTTP Request.
func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "The answer is: %d", cadep.Answer())
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
	f.Build.BuildEnvs = []fn.Env{
		// Our proxy first; it returns 404 for other modules so that they are
		// fetched from the public proxy. A TLS error is not retried there.
		{Name: ptr("GOPROXY"), Value: ptr(proxy.url + ",https://proxy.golang.org")},
		{Name: ptr("GOSUMDB"), Value: ptr("off")},
	}
	return f
}

// newTestCA returns a new self-signed CA certificate, its key and its PEM.
func newTestCA(t *testing.T, cn string) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          randSN(),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, 1),
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
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// newServerCert returns a TLS certificate for localhost and 127.0.0.1 signed
// by the given CA.
func newServerCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: randSN(),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 1),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// Generates self-signed certificate used for our private git repository.
func createCertificate(t *testing.T) string {
	dir := t.TempDir()

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	ski := sha1.Sum(x509.MarshalPKCS1PublicKey(&certPrivKey.PublicKey))

	cert := &x509.Certificate{
		BasicConstraintsValid: true,
		IsCA:                  true,
		SerialNumber:          randSN(),
		// openssl hash of this subject is 85c05568
		// do not update the subject without also updating the hash referred from another places (e.g. Dockerfile)
		// See also: https://github.com/paketo-buildpacks/ca-certificates/blob/v1.0.1/cacerts/certs.go#L132
		Subject: pkix.Name{
			CommonName: "git-private.localtest.me",
		},
		DNSNames:     []string{"git-private.localtest.me"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, 1),
		SubjectKeyId: ski[:],
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &certPrivKey.PublicKey, certPrivKey)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		t.Fatal(err)
	}

	certPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM.Bytes(), 0444)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dir, "key.pem"), certPrivKeyPEM.Bytes(), 0400)
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

var maxSN *big.Int = new(big.Int).Lsh(big.NewInt(1), 159)

func randSN() *big.Int {
	i, err := rand.Int(rand.Reader, maxSN)
	if err != nil {
		panic(err)
	}
	return i
}

// Builds a s2i Golang builder that trusts to our self-signed certificate (see createCertificate).
func buildPatchedS2IBuilder(ctx context.Context, t *testing.T, certDir string) string {
	tag := "registry.localtest.me/go-toolset:test"
	dockerfile := `FROM registry.access.redhat.com/ubi8/go-toolset:latest
COPY 85c05568.0 /etc/pki/ca-trust/source/anchors/
USER 0:0
RUN update-ca-trust
USER 1001:0
`
	return buildPatchedBuilder(ctx, t, tag, dockerfile, certDir)
}

// Builds a tiny paketo builder that trusts to our self-signed certificate (see createCertificate).
func buildPatchedBuildpackBuilder(ctx context.Context, t *testing.T, certDir string) string {
	tag := "registry.localtest.me/builder-jammy-tin:test"
	dockerfile := `FROM ghcr.io/knative/builder-jammy-tiny:v2
COPY 85c05568.0 /etc/ssl/certs/
`
	return buildPatchedBuilder(ctx, t, tag, dockerfile, certDir)
}

// Builds an image with specified tag from specified dockerfile.
// This function also injects self-signed as "85c05568.0" into the build context.
func buildPatchedBuilder(ctx context.Context, t *testing.T, tag, dockerfile, certDir string) string {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	var buff bytes.Buffer
	tw := tar.NewWriter(&buff)

	err = tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfile)),
		Mode: 0644,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tw.Write([]byte(dockerfile))
	if err != nil {
		t.Fatal(err)
	}

	cb, err := os.ReadFile(filepath.Join(certDir, "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	err = tw.WriteHeader(&tar.Header{
		Name: "85c05568.0",
		Size: int64(len(cb)),
		Mode: 0644,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tw.Write(cb)
	if err != nil {
		t.Fatal(err)
	}

	err = tw.Close()
	if err != nil {
		t.Fatal(err)
	}

	ibo := client.ImageBuildOptions{
		Tags: []string{tag},
	}
	ibr, err := cli.ImageBuild(ctx, &buff, ibo)
	if err != nil {
		t.Fatal(err)
	}
	defer ibr.Body.Close()

	fd := os.Stderr.Fd()
	isTerminal := term.IsTerminal(int(fd))
	err = jsonmessage.DisplayJSONMessagesStream(ibr.Body, os.Stderr, fd, isTerminal, nil)
	if err != nil {
		t.Fatal(err)
	}

	rc, err := cli.ImagePush(ctx, tag, client.ImagePushOptions{RegistryAuth: "e30="})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	return tag
}

// This sets up a private git repository for testing.
// The repository url is https://git-private.localtest.me/foo.git, and it is protected by basic authentication.
// The credentials are developer:nbusr123.
func servePrivateGit(ctx context.Context, t *testing.T, certDir string) {
	const (
		name  = "git-private"
		host  = "git-private.localtest.me"
		image = "ghcr.io/matejvasek/git-private:latest"
	)

	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	ns := coreV1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = k8sClient.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = k8sClient.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	cert, err := os.ReadFile(filepath.Join(certDir, "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(filepath.Join(certDir, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}

	secret := coreV1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
		},
		Immutable: ptr(true),
		Data: map[string][]byte{
			coreV1.TLSCertKey:       cert,
			coreV1.TLSPrivateKeyKey: key,
		},
		Type: coreV1.SecretTypeTLS,
	}

	_, err = k8sClient.CoreV1().Secrets(name).Create(ctx, &secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	pod := coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
			Labels:    map[string]string{"app.kubernetes.io/name": name},
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{
				{
					Name:  name,
					Image: image,
					Ports: []coreV1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
						},
					},
				},
			},
		},
	}
	_, err = k8sClient.CoreV1().Pods(name).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	svc := coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": name,
			},
			Ports: []coreV1.ServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
			Type: coreV1.ServiceTypeClusterIP,
		},
	}
	_, err = k8sClient.CoreV1().Services(name).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ingress := v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
		},
		Spec: v1.IngressSpec{
			IngressClassName: ptr("contour-external"),
			DefaultBackend:   nil,
			TLS: []v1.IngressTLS{
				{
					Hosts:      []string{host},
					SecretName: name,
				},
			},
			Rules: []v1.IngressRule{
				{
					Host: host,
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{Paths: []v1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: ptr(v1.PathTypePrefix),
								Backend: v1.IngressBackend{
									Service: &v1.IngressServiceBackend{
										Name: name,
										Port: v1.ServiceBackendPort{
											Name: "http",
										},
									},
								},
							},
						}},
					},
				},
			},
		},
	}
	_, err = k8sClient.NetworkingV1().Ingresses(name).Create(ctx, &ingress, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](val T) *T {
	return &val
}
