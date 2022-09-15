package http_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	fnhttp "knative.dev/kn-plugin-func/http"
	. "knative.dev/kn-plugin-func/testing"
)

const inClusterHostName = "a-testing-service.a-testing-namespace.svc"

func TestCustomCA(t *testing.T) {
	defer WithEnvVar(t, "SOCAT_IMAGE", "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root")()

	var err error
	inClusterAddr, inClusterCA := startServer(t, inClusterHostName)
	localhostAddr, localhostCA := startServer(t, "localhost")

	mockSelectCA := func(ctx context.Context, serverName string) (*x509.Certificate, error) {

		if serverName == inClusterHostName {
			return inClusterCA, nil
		}
		if serverName == "localhost" {
			return localhostCA, nil
		}
		return nil, nil
	}

	mockInCusterDialer := mockInClusterDialer{
		backingAddr: inClusterAddr,
	}

	tr := fnhttp.NewRoundTripper(
		fnhttp.WithSelectCA(mockSelectCA),
		fnhttp.WithInClusterDialer(mockInCusterDialer))
	defer tr.Close()

	client := http.Client{Transport: tr}

	_, p, err := net.SplitHostPort(localhostAddr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(fmt.Sprintf("https://localhost:%s", p))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("https://%s:5000/v2/", inClusterHostName))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

}

type mockInClusterDialer struct {
	backingAddr string
}

func (m mockInClusterDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	hostname, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if hostname == inClusterHostName {
		return net.Dial(network, m.backingAddr)
	}
	return net.Dial(network, addr)
}

func (m mockInClusterDialer) Close() error {
	return nil
}

func startServer(t *testing.T, hostname string) (addr string, ca *x509.Certificate) {
	randReader := rand.Reader

	caPublicKey, caPrivateKey, err := ed25519.GenerateKey(randReader)
	if err != nil {
		t.Fatal(err)
	}

	ca = &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: hostname,
		},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost", hostname},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		ExtraExtensions:       []pkix.Extension{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(randReader, ca, ca, caPublicKey, caPrivateKey)
	if err != nil {
		t.Fatal()
	}

	ca, err = x509.ParseCertificate(caBytes)
	if err != nil {
		t.Fatal(err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{caBytes},
		PrivateKey:  caPrivateKey,
		Leaf:        ca,
	}

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = listener.Addr().String()

	handler := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
	})

	server := http.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			ServerName:   hostname,
			Certificates: []tls.Certificate{cert},
		},
	}
	t.Cleanup(func() {
		server.Close()
	})

	go func() {
		_ = server.ServeTLS(listener, "", "")
	}()
	return
}
