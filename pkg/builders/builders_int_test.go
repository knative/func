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
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"knative.dev/func/pkg/builders/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func TestPrivateGitRepository(t *testing.T) {
	t.Skip("tested functionality not implemented yet")

	ctx, cancel := context.WithCancel(context.Background())
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

	builderImage := buildPatchedBuilder(ctx, t, certDir)
	t.Log("builder image:", builderImage)

	servePrivateGit(ctx, t, certDir)
	t.Log("git server initiated")

	select {
	case <-time.After(time.Second * 5):
		break
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	builder := buildpacks.NewBuilder(buildpacks.WithVerbose(true))
	f, err := fn.NewFunction(filepath.Join("testdata", "go-fn-with-private-deps"))
	if err != nil {
		t.Fatal(err)
	}
	f.Build.Image = "localhost:50000/go-app:test"
	f.Build.Builder = "pack"
	f.Build.BuilderImages = map[string]string{"pack": builderImage}
	err = builder.Build(ctx, f, nil)
	if err != nil {
		t.Fatal(err)
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
		SerialNumber: randSN(),
		// openssl hash of this subject is 712d4c9d
		// do not update the subject without also updating the hash referred from another places (e.g. Dockerfile)
		// See also: https://github.com/paketo-buildpacks/ca-certificates/blob/v1.0.1/cacerts/certs.go#L132
		Subject: pkix.Name{
			CommonName: "git-private.127.0.0.1.sslip.io",
		},
		DNSNames:     []string{"git-private.127.0.0.1.sslip.io"},
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

// Builds a tiny paketo builder that trusts to our self-signed certificate (see createCertificate).
func buildPatchedBuilder(ctx context.Context, t *testing.T, certDir string) string {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}

	dockerfile := `FROM ghcr.io/knative/builder-jammy-base:latest
COPY 712d4c9d.0 /etc/ssl/certs/
`

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
		Name: "712d4c9d.0",
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

	tag := "localhost:50000/tiny-builder:test"
	ibo := types.ImageBuildOptions{
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

	rc, err := cli.ImagePush(ctx, tag, image.PushOptions{RegistryAuth: "e30="})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	return tag
}

// This sets up a private git repository for testing.
// The repository url is https://git-private.127.0.0.1.sslip.io/foo.git, and it is protected by basic authentication.
// The credentials are developer:nbusr123.
func servePrivateGit(ctx context.Context, t *testing.T, certDir string) {
	const (
		name  = "git-private"
		host  = "git-private.127.0.0.1.sslip.io"
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
