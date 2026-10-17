//go:build integration

package builders_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/moby/client"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
			Name:         "pack",
			Scaffolder:   buildpacks.NewScaffolder(true),
			Builder:      buildpacks.NewBuilder(buildpacks.WithVerbose(true)),
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

// TestInt_BuildCACertFile tests that CA certificate bundles are properly used
// during builds for both pack and s2i builders. This test verifies that:
// - pack builder uses Paketo ca-certificates binding
// - s2i builder uses environment variables for CA bundle
// - builds can access resources with custom CA certificates
func TestInt_BuildCACertFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping TestInt_BuildCACertFile on non-Linux systems due to cluster networking limitations")
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

	// Create a self-signed CA certificate for testing
	certDir := createCertificate(t)
	t.Log("certDir:", certDir)

	// Use the cert.pem as our CA bundle file
	caBundlePath := filepath.Join(certDir, "cert.pem")

	// Verify CA bundle file exists
	if _, err := os.Stat(caBundlePath); err != nil {
		t.Fatalf("CA bundle file not found: %v", err)
	}

	tmpDir := t.TempDir()

	// Copy test function
	src := filepath.Join("testdata", "go-fn-with-private-deps")
	dst := filepath.Join(tmpDir, "go-fn-with-ca-test")

	err := copyDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(dst)
	if err != nil {
		t.Fatal(err)
	}

	// Set the CA bundle file in the function spec
	f.Build.BuildCACertFile = caBundlePath

	testCases := []struct {
		Name         string
		Scaffolder   fn.Scaffolder
		Builder      fn.Builder
		BuilderImage func(ctx context.Context, t *testing.T, certDir string) string
	}{
		{
			Name:         "pack",
			Scaffolder:   buildpacks.NewScaffolder(true),
			Builder:      buildpacks.NewBuilder(buildpacks.WithVerbose(true)),
			BuilderImage: buildPatchedBuildpackBuilder,
		},
		{
			Name:         "s2i",
			Scaffolder:   s2i.NewScaffolder(true),
			Builder:      s2i.NewBuilder(s2i.WithVerbose(true)),
			BuilderImage: buildPatchedS2IBuilder,
		},
	}

	for _, tt := range testCases {
		var f = f
		t.Run(tt.Name, func(t *testing.T) {
			f.Build.Image = "registry.localtest.me/go-app:ca-test-" + tt.Name
			f.Build.Builder = tt.Name
			f.Build.BuilderImages = map[string]string{
				tt.Name: tt.BuilderImage(ctx, t, certDir),
			}
			f.Build.BuildCACertFile = caBundlePath

			// Scaffold the function
			err = tt.Scaffolder.Scaffold(ctx, f, "")
			if err != nil {
				t.Fatal(err)
			}

			// Build the function - this should succeed if CA bundle is properly used
			err = tt.Builder.Build(ctx, f, nil)
			if err != nil {
				t.Fatalf("%s builder failed to build with CA bundle: %v", tt.Name, err)
			}

			t.Logf("%s builder successfully built with CA bundle", tt.Name)
		})
	}
}

// TestInt_BuildCACertFile_RelativePath tests that relative paths to CA bundles
// are properly resolved relative to the function root
func TestInt_BuildCACertFile_RelativePath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux systems due to cluster networking limitations")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Create a self-signed CA certificate
	certDir := createCertificate(t)
	caBundleSource := filepath.Join(certDir, "cert.pem")

	tmpDir := t.TempDir()

	// Copy test function
	src := filepath.Join("testdata", "go-fn-with-private-deps")
	dst := filepath.Join(tmpDir, "go-fn-relative-ca")

	err := copyDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	// Copy CA bundle to function root with a relative name
	caBundleInFunc := filepath.Join(dst, "my-ca-bundle.crt")
	caData, err := os.ReadFile(caBundleSource)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(caBundleInFunc, caData, 0644)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(dst)
	if err != nil {
		t.Fatal(err)
	}

	// Use RELATIVE path
	f.Build.BuildCACertFile = "my-ca-bundle.crt"

	// Test with pack builder
	scaffolder := buildpacks.NewScaffolder(true)
	builder := buildpacks.NewBuilder(buildpacks.WithVerbose(true))

	f.Build.Image = "registry.localtest.me/go-app:ca-relative-test"
	f.Build.Builder = "pack"
	f.Build.BuilderImages = map[string]string{
		"pack": buildPatchedBuildpackBuilder(ctx, t, certDir),
	}

	err = scaffolder.Scaffold(ctx, f, "")
	if err != nil {
		t.Fatal(err)
	}

	err = builder.Build(ctx, f, nil)
	if err != nil {
		t.Fatalf("pack builder failed with relative CA bundle path: %v", err)
	}

	t.Log("pack builder successfully built with relative CA bundle path")
}

// TestInt_BuildCACertFile_MissingFile tests that builds fail gracefully
// when CA bundle file doesn't exist
func TestInt_BuildCACertFile_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test function
	src := filepath.Join("testdata", "go-fn-with-private-deps")
	dst := filepath.Join(tmpDir, "go-fn-missing-ca")

	err := copyDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(dst)
	if err != nil {
		t.Fatal(err)
	}

	// Point to non-existent CA bundle
	f.Build.BuildCACertFile = "/nonexistent/ca-bundle.crt"

	scaffolder := buildpacks.NewScaffolder(true)
	builder := buildpacks.NewBuilder(buildpacks.WithVerbose(true))

	f.Build.Image = "registry.localtest.me/go-app:ca-missing-test"
	f.Build.Builder = "pack"

	err = scaffolder.Scaffold(context.Background(), f, "")
	if err != nil {
		t.Fatal(err)
	}

	// Build should fail with clear error about missing CA bundle
	err = builder.Build(context.Background(), f, nil)
	if err == nil {
		t.Fatal("expected build to fail with missing CA bundle, but it succeeded")
	}

	if !strings.Contains(err.Error(), "CA bundle file not found") {
		t.Fatalf("expected error message about CA bundle not found, got: %v", err)
	}

	t.Log("build correctly failed with missing CA bundle file")
}
