//go:build !integration
// +build !integration

package docker_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	api "github.com/docker/docker/api/types/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	regTypes "github.com/google/go-containerregistry/pkg/v1/types"
	"gotest.tools/v3/assert"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
)

func TestGetRegistry(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "default registry",
			arg:  "docker.io/mysamplefunc:latest",
			want: "index.docker.io",
		},
		{
			name: "long-form nested url",
			arg:  "myregistry.io/myorg/myuser/myfunctions/mysamplefunc:latest",
			want: "myregistry.io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := docker.GetRegistry(tt.arg); got != tt.want {
				t.Errorf("GetRegistry() = %v, want %v", got, tt.want)
			}
		})
	}
}

const (
	testUser            = "testuser"
	testPwd             = "testpwd"
	registryHostname    = "my.testing.registry"
	functionImageRemote = registryHostname + "/testuser/func:latest"

	imageTarball    = "testdata/image.tar"
	imageID         = "sha256:1fb61f35700f47e1e868f8b26fdd777016f96bb1b4b0b0e623efac39eb30d12e"
	imageRepoDigest = "sha256:00af51d125f3092e157a7f8a717029412dc9d266c017e89cecdfeccb4cc3d7a7"
)

var testCredProvider = docker.CredentialsProvider(func(ctx context.Context, registry string) (docker.Credentials, error) {
	return docker.Credentials{
		Username: testUser,
		Password: testPwd,
	}, nil
})

func TestPush(t *testing.T) {

	tests := []struct {
		Name       string
		DaemonPush bool
	}{
		{Name: "daemon push", DaemonPush: true},
		{Name: "non daemon push"},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
			defer cancel()

			// in memory network emulation
			connections := conns(make(chan net.Conn))

			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
			transport.DialContext = connections.DialContext

			serveRegistry(t, connections)

			dockerClient := newMockPusherDockerClient()

			pushReachable := func(ctx context.Context, ref string, options api.PushOptions) (io.ReadCloser, error) {
				if ref != functionImageRemote {
					return nil, fmt.Errorf("unexpected ref")
				}

				var err error

				authData, err := base64.StdEncoding.DecodeString(options.RegistryAuth)
				if err != nil {
					return nil, err
				}

				authStruct := struct {
					Username, Password string
				}{}

				dec := json.NewDecoder(bytes.NewReader(authData))

				err = dec.Decode(&authStruct)
				if err != nil {
					return nil, err
				}

				remoteOpts := []remote.Option{
					remote.WithTransport(transport),
					remote.WithAuth(&authn.Basic{
						Username: authStruct.Username,
						Password: authStruct.Password,
					}),
				}
				tag, err := name.NewTag(ref)
				if err != nil {
					return nil, err
				}
				img, err := tarball.ImageFromPath(imageTarball, &tag)
				if err != nil {
					return nil, err
				}

				err = remote.Write(tag, img, remoteOpts...)
				if err != nil {
					return nil, err
				}
				is, err := img.Size()
				if err != nil {
					return nil, err
				}
				return io.NopCloser(strings.NewReader(`{
    "status":  "latest: digest: ` + imageRepoDigest + ` size: ` + strconv.FormatInt(is, 10) + `"
}
`)), nil
			}

			pushUnreachable := func(ctx context.Context, ref string, options api.PushOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(`{"errorDetail": {"message": "...no such host..."}}`)), nil
			}

			if tt.DaemonPush {
				dockerClient.imagePush = pushReachable
			} else {
				dockerClient.imagePush = pushUnreachable
			}

			dockerClient.imageInspect = func(ctx context.Context, s string) (types.ImageInspect, []byte, error) {
				return types.ImageInspect{ID: imageID}, []byte{}, nil
			}

			dockerClient.imageSave = func(ctx context.Context, tags []string) (io.ReadCloser, error) {
				if slices.Equal(tags, []string{functionImageRemote}) {
					f, err := os.Open(imageTarball)
					if err != nil {
						return nil, err
					}
					return f, nil
				}
				return nil, fmt.Errorf("unexpected tags")
			}

			var closeCalledOnMock bool
			dockerClient.close = func() error {
				closeCalledOnMock = true
				return nil
			}

			dockerClientFactory := func() (docker.PusherDockerClient, error) {
				return dockerClient, nil
			}

			pusher := docker.NewPusher(
				docker.WithTransport(transport),
				docker.WithCredentialsProvider(testCredProvider),
				docker.WithPusherDockerClientFactory(dockerClientFactory),
			)

			f := fn.Function{
				Build: fn.BuildSpec{
					Image: functionImageRemote,
				},
			}

			remoteOpts := []remote.Option{
				remote.WithTransport(transport),
				remote.WithAuth(&authn.Basic{
					Username: testUser,
					Password: testPwd,
				}),
			}

			_, err := pusher.Push(ctx, f)
			if err != nil {
				t.Fatal(err)
			}

			r, err := name.NewRegistry(registryHostname)
			if err != nil {
				t.Fatal(err)
			}

			c, err := remote.Catalog(ctx, r, remoteOpts...)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(c, []string{"testuser/func"}) {
				t.Error("unexpected catalog content")
			}

			ref := name.MustParseReference(functionImageRemote)

			desc, err := remote.Get(ref, remoteOpts...)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, desc.MediaType, regTypes.DockerManifestList)

			img, err := remote.Image(ref, remoteOpts...)
			if err != nil {
				t.Fatal(err)
			}

			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, d.String(), imageRepoDigest)

			if !closeCalledOnMock {
				t.Error("The Close() function has not been called on the Docker API Client.")
			}
		})
	}
}

func newMockPusherDockerClient() *mockPusherDockerClient {
	return &mockPusherDockerClient{
		imagePush: func(ctx context.Context, ref string, options api.PushOptions) (io.ReadCloser, error) {
			return nil, fmt.Errorf(" imagePush not implemented")
		},
		imageSave: func(ctx context.Context, strings []string) (io.ReadCloser, error) {
			return nil, fmt.Errorf("imageSave not implemented")
		},
		imageInspect: func(ctx context.Context, s string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, fmt.Errorf("imageInspect not implemented")
		},
		negotiateAPIVersion: func(ctx context.Context) {},
		close:               func() error { return nil },
	}
}

type mockPusherDockerClient struct {
	negotiateAPIVersion func(ctx context.Context)
	imagePush           func(ctx context.Context, ref string, options api.PushOptions) (io.ReadCloser, error)
	imageSave           func(ctx context.Context, strings []string) (io.ReadCloser, error)
	imageInspect        func(ctx context.Context, s string) (types.ImageInspect, []byte, error)
	close               func() error
}

func (m *mockPusherDockerClient) NegotiateAPIVersion(ctx context.Context) {
	m.negotiateAPIVersion(ctx)
}

func (m *mockPusherDockerClient) ImageSave(ctx context.Context, strings []string) (io.ReadCloser, error) {
	return m.imageSave(ctx, strings)
}

func (m *mockPusherDockerClient) ImageLoad(ctx context.Context, reader io.Reader, b bool) (api.LoadResponse, error) {
	panic("implement me")
}

func (m *mockPusherDockerClient) ImageTag(ctx context.Context, s string, s2 string) error {
	panic("implement me")
}

func (m *mockPusherDockerClient) ImageInspectWithRaw(ctx context.Context, s string) (types.ImageInspect, []byte, error) {
	return m.imageInspect(ctx, s)
}

func (m *mockPusherDockerClient) ImagePush(ctx context.Context, ref string, options api.PushOptions) (io.ReadCloser, error) {
	return m.imagePush(ctx, ref, options)
}

func (m *mockPusherDockerClient) ImageHistory(context.Context, string) ([]api.HistoryResponseItem, error) {
	return nil, errors.New("the ImageHistory() function is not implemented")
}

func (m *mockPusherDockerClient) Close() error {
	return m.close()
}

func serveRegistry(t *testing.T, l net.Listener) {

	caPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caPublicKey := &caPrivateKey.PublicKey

	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: registryHostname,
		},
		DNSNames:              []string{registryHostname},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Minute * 10),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		ExtraExtensions:       []pkix.Extension{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, caPublicKey, caPrivateKey)
	if err != nil {
		t.Fatal(err)
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

	server := http.Server{
		Handler: withAuth(registry.New(
			registry.Logger(log.New(io.Discard, "", 0)))),
		TLSConfig: &tls.Config{
			ServerName:   registryHostname,
			Certificates: []tls.Certificate{cert},
		},
		// The line below disables HTTP/2.
		// See: https://github.com/google/go-containerregistry/issues/1210
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	go func() {
		_ = server.ServeTLS(l, "", "")
	}()
	t.Cleanup(func() {
		server.Close()
	})
}

// middleware for basic auth
func withAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if ok && user == testUser && pass == testPwd {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Add("WWW-Authenticate", "basic")
		w.WriteHeader(401)
		fmt.Fprintln(w, "Unauthorised.")
	})
}

type conns chan net.Conn

func (c conns) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if addr == registryHostname+":443" {

		pr0, pw0 := io.Pipe()
		pr1, pw1 := io.Pipe()

		c <- conn{pr0, pw1}

		return conn{pr1, pw0}, nil
	}
	return (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext(ctx, network, addr)
}

func (c conns) Accept() (net.Conn, error) {
	con, ok := <-c
	if !ok {
		return nil, net.ErrClosed
	}
	return con, nil
}

func (c conns) Close() error {
	close(c)
	return nil
}

func (c conns) Addr() net.Addr {
	return addr{}
}

type conn struct {
	pr *io.PipeReader
	pw *io.PipeWriter
}

type addr struct{}

func (a addr) Network() string {
	return "mock-addr"
}

func (a addr) String() string {
	return "mock-addr"
}

func (c conn) Read(b []byte) (n int, err error) {
	return c.pr.Read(b)
}

func (c conn) Write(b []byte) (n int, err error) {
	return c.pw.Write(b)
}

func (c conn) Close() error {
	var err error

	err = c.pr.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}

	err = c.pw.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}

	return nil
}

func (c conn) LocalAddr() net.Addr {
	return addr{}
}

func (c conn) RemoteAddr() net.Addr {
	return addr{}
}

func (c conn) SetDeadline(t time.Time) error { return nil }

func (c conn) SetReadDeadline(t time.Time) error { return nil }

func (c conn) SetWriteDeadline(t time.Time) error { return nil }
