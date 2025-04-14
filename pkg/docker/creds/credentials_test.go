//go:build !integration
// +build !integration

package creds_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker-credential-helpers/credentials"

	"knative.dev/func/pkg/docker"
	"knative.dev/func/pkg/docker/creds"
	. "knative.dev/func/pkg/testing"
)

var homeTempDir string

func TestMain(m *testing.M) {
	// github.com/containers/image only computes $HOME once so we need to set it
	// globally for all the tests
	var err error
	homeTempDir, err = os.MkdirTemp("", "")
	if err != nil {
		panic("failed to create tempdir" + err.Error())
	}
	os.Setenv(testHomeEnvName(), homeTempDir)
	if runtime.GOOS == "linux" {
		os.Setenv("XDG_CONFIG_HOME", filepath.Join(homeTempDir, ".config"))
	}

	os.Exit(m.Run())
}

func Test_registryEquals(t *testing.T) {
	tests := []struct {
		name string
		urlA string
		urlB string
		want bool
	}{
		{"no port matching host", "quay.io", "quay.io", true},
		{"non-matching host added sub-domain", "sub.quay.io", "quay.io", false},
		{"non-matching host different sub-domain", "sub.quay.io", "sub3.quay.io", false},
		{"localhost", "localhost", "localhost", true},
		{"localhost with standard ports", "localhost:80", "localhost:443", false},
		{"localhost with matching port", "https://localhost:1234", "http://localhost:1234", true},
		{"localhost with match by default port 80", "http://localhost", "localhost:80", true},
		{"localhost with match by default port 443", "https://localhost", "localhost:443", true},
		{"localhost with mismatch by non-default port 5000", "https://localhost", "localhost:5000", false},
		{"localhost with match by empty ports", "https://localhost", "http://localhost", true},
		{"docker.io matching host https", "https://docker.io", "docker.io", true},
		{"docker.io matching host http", "http://docker.io", "docker.io", true},
		{"docker.io with path", "docker.io/v1/", "docker.io", true},
		{"docker.io with protocol and path", "https://docker.io/v1/", "docker.io", true},
		{"docker.io with subdomain index.", "https://index.docker.io/v1/", "docker.io", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := creds.RegistryEquals(tt.urlA, tt.urlB); got != tt.want {
				t.Errorf("to2ndLevelDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAuth(t *testing.T) {

	const (
		uname        = "testuser"
		pwd          = "testpwd"
		incorrectPwd = "badpwd"
	)

	localhost, localhostTLS, cert := startServer(t, uname, pwd)

	_, portTLS, err := net.SplitHostPort(localhostTLS)
	if err != nil {
		t.Fatal(err)
	}

	nonLocalhostTLS := "test.io:" + portTLS

	type args struct {
		ctx      context.Context
		username string
		password string
		registry string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "correct credentials localhost no-TLS",
			args: args{
				ctx:      context.Background(),
				username: uname,
				password: pwd,
				registry: localhost,
			},
			wantErr: false,
		},
		{
			name: "correct credentials localhost",
			args: args{
				ctx:      context.Background(),
				username: uname,
				password: pwd,
				registry: localhostTLS,
			},
			wantErr: false,
		},
		{
			name: "correct credentials non-localhost",
			args: args{
				ctx:      context.Background(),
				username: uname,
				password: pwd,
				registry: nonLocalhostTLS,
			},
			wantErr: false,
		},
		{
			name: "incorrect credentials localhost no-TLS",
			args: args{
				ctx:      context.Background(),
				username: uname,
				password: incorrectPwd,
				registry: localhost,
			},
			wantErr: true,
		},
		{
			name: "incorrect credentials localhost",
			args: args{
				ctx:      context.Background(),
				username: uname,
				password: incorrectPwd,
				registry: localhostTLS,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := docker.Credentials{
				Username: tt.args.username,
				Password: tt.args.password,
			}
			// create trusted certificates pool and add our certificate
			certPool := x509.NewCertPool()
			certPool.AddCert(cert)

			// client transport with the certificate
			transport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: certPool,
				},
			}

			dialer := &net.Dialer{}

			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				h, p, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				if h == "test.io" {
					h = "localhost"
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(h, p))
			}
			if err := creds.CheckAuth(tt.args.ctx, tt.args.registry+"/someorg/someimage:sometag", c, transport); (err != nil) != tt.wantErr {
				t.Errorf("CheckAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckAuthEmptyCreds(t *testing.T) {

	localhost, _, _ := startServer(t, "", "")
	err := creds.CheckAuth(context.Background(), localhost+"/someorg/someimage:sometag", docker.Credentials{}, http.DefaultTransport)
	if err != nil {
		t.Error(err)
	}
}

// generate Certificates
func generateCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	var randReader = rand.Reader

	caPublicKey, caPrivateKey, err := ed25519.GenerateKey(randReader)
	if err != nil {
		t.Fatal(err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost", "test.io"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		ExtraExtensions:       []pkix.Extension{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(randReader, caTemplate, caTemplate, caPublicKey, caPrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	ca, err := x509.ParseCertificate(caBytes)
	if err != nil {
		t.Fatal(err)
	}

	tls := tls.Certificate{
		Certificate: [][]byte{caBytes},
		PrivateKey:  caPrivateKey,
		Leaf:        ca,
	}
	return tls, ca
}

func startServer(t *testing.T, uname, pwd string) (addr, addrTLS string, ca *x509.Certificate) {
	// create a custom handler function
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no authentication required, empty creds
		if uname == "" || pwd == "" {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			return
		}

		w.Header().Add("WWW-Authenticate", "basic")
		if u, p, ok := r.BasicAuth(); ok {
			if u == uname && p == pwd {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	})

	// Setup certificates
	// tls Cert for the TLS server (has ca as Leaf)
	// x509 certificate which is its own CA for client
	tlsCert, ca := generateCert(t)

	// create Server config
	server := http.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			ServerName: "localhost",
			// with the TLS certificate
			Certificates: []tls.Certificate{tlsCert},
		},
	}

	// non-TLS listener
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	// TLS listener
	listenerTLS, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = listener.Addr().String()
	addrTLS = listenerTLS.Addr().String()

	// listen for requests
	go func() {
		err := server.ServeTLS(listenerTLS, "", "")
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	go func() {
		err := server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	// shutdown servers at cleanup
	t.Cleanup(func() {
		err := server.Shutdown(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	})

	return
}

const (
	dockerIoUser    = "testUser1"
	dockerIoUserPwd = "goodPwd1"
	quayIoUser      = "testUser2"
	quayIoUserPwd   = "goodPwd2"
)

type Credentials = docker.Credentials

func TestNewCredentialsProvider(t *testing.T) {
	helperWithQuayIO := newInMemoryHelper()

	err := helperWithQuayIO.Add(&credentials.Credentials{
		ServerURL: "quay.io",
		Username:  quayIoUser,
		Secret:    quayIoUserPwd,
	})
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		promptUser        creds.CredentialsCallback
		verifyCredentials creds.VerifyCredentialsCallback
		additionalLoaders []creds.CredentialsCallback
		registry          string
		setUpEnv          setUpEnv
	}
	tests := []struct {
		name string
		args args
		want Credentials
	}{
		{
			name: "test user callback correct password on first try",
			args: args{
				promptUser:        correctPwdCallback,
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "test user callback correct password on second try",
			args: args{
				promptUser:        pwdCbkFirstWrongThenCorrect(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "get quay-io credentials with func config populated",
			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "quay.io",
				setUpEnv:          withPopulatedFuncAuthConfig,
			},
			want: Credentials{Username: quayIoUser, Password: quayIoUserPwd},
		},
		{
			name: "get docker-io credentials with func config populated",
			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				setUpEnv:          withPopulatedFuncAuthConfig,
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "get quay-io credentials with docker config populated",
			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "quay.io",
				setUpEnv: all(
					withPopulatedDockerAuthConfig,
					setUpMockHelper("docker-credential-mock", helperWithQuayIO)),
			},
			want: Credentials{Username: quayIoUser, Password: quayIoUserPwd},
		},
		{
			name: "get docker-io credentials with docker config populated",
			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				setUpEnv: all(
					withPopulatedDockerAuthConfig,
					setUpMockHelper("docker-credential-mock", newInMemoryHelper())),
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "get docker-io credentials from custom loader",
			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				additionalLoaders: []creds.CredentialsCallback{correctPwdCallback},
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHomeDir(t)
			if tt.args.setUpEnv != nil {
				tt.args.setUpEnv(t)
			}

			credentialsProvider := creds.NewCredentialsProvider(
				testConfigPath(t),
				creds.WithPromptForCredentials(tt.args.promptUser),
				creds.WithVerifyCredentials(tt.args.verifyCredentials),
				creds.WithAdditionalCredentialLoaders(tt.args.additionalLoaders...))
			got, err := credentialsProvider(context.Background(), tt.args.registry+"/someorg/someimage:sometag")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("credentialsProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewCredentialsProviderEmptyCreds(t *testing.T) {
	resetHomeDir(t)

	credentialsProvider := creds.NewCredentialsProvider(testConfigPath(t), creds.WithVerifyCredentials(func(ctx context.Context, image string, credentials docker.Credentials) error {
		if image == "localhost:5555/someorg/someimage:sometag" && credentials == (docker.Credentials{}) {
			return nil
		}
		t.Fatal("unreachable")
		return nil
	}))
	c, err := credentialsProvider(context.Background(), "localhost:5555/someorg/someimage:sometag")
	if err != nil {
		t.Error(err)
	}
	if c != (docker.Credentials{}) {
		t.Error("unexpected credentials")
	}
}

func TestCredentialsProviderSavingFromUserInput(t *testing.T) {
	resetHomeDir(t)

	helper := newInMemoryHelper()
	setUpMockHelper("docker-credential-mock", helper)(t)

	var pwdCbkInvocations int
	pwdCbk := func(r string) (Credentials, error) {
		pwdCbkInvocations++
		return correctPwdCallback(r)
	}

	chooseNoStore := func(available []string) (string, error) {
		if len(available) < 1 {
			t.Errorf("this should have been invoked with non empty list")
		}
		return "", nil
	}
	chooseMockStore := func(available []string) (string, error) {
		if len(available) < 1 {
			t.Errorf("this should have been invoked with non empty list")
		}
		return "docker-credential-mock", nil
	}
	shallNotBeInvoked := func(available []string) (string, error) {
		t.Fatal("this choose helper callback shall not be invoked")
		return "", errors.New("this callback shall not be invoked")
	}

	credentialsProvider := creds.NewCredentialsProvider(
		testConfigPath(t),
		creds.WithPromptForCredentials(pwdCbk),
		creds.WithVerifyCredentials(correctVerifyCbk),
		creds.WithPromptForCredentialStore(chooseNoStore))
	_, err := credentialsProvider(context.Background(), "docker.io/someorg/someimage:sometag")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// now credentials should not be saved because no helper was provided
	l, err := helper.List()
	if err != nil {
		t.Fatal(err)
	}
	credsInStore := len(l)
	if credsInStore != 0 {
		t.Errorf("expected to have zero credentials in store, but has: %d", credsInStore)
	}
	credentialsProvider = creds.NewCredentialsProvider(
		testConfigPath(t),
		creds.WithPromptForCredentials(pwdCbk),
		creds.WithVerifyCredentials(correctVerifyCbk),
		creds.WithPromptForCredentialStore(chooseMockStore))
	_, err = credentialsProvider(context.Background(), "docker.io/someorg/someimage:sometag")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	if pwdCbkInvocations != 2 {
		t.Errorf("the pwd callback should have been invoked exactly twice but was invoked %d time", pwdCbkInvocations)
	}

	// now credentials should be saved in the mock secure store
	l, err = helper.List()
	if err != nil {
		t.Fatal(err)
	}
	credsInStore = len(l)
	if len(l) != 1 {
		t.Errorf("expected to have exactly one credentials in store, but has: %d", credsInStore)
	}
	credentialsProvider = creds.NewCredentialsProvider(
		testConfigPath(t),
		creds.WithPromptForCredentials(pwdCbkThatShallNotBeCalled(t)),
		creds.WithVerifyCredentials(correctVerifyCbk),
		creds.WithPromptForCredentialStore(shallNotBeInvoked))
	_, err = credentialsProvider(context.Background(), "docker.io/someorg/someimage:sometag")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
}

// TestCredentialsWithoutHome ensures that credentialProvider works when HOME is
// not set or config is empty
func TestCredentialsWithoutHome(t *testing.T) {
	type args struct {
		promptUser        creds.CredentialsCallback
		verifyCredentials creds.VerifyCredentialsCallback
		registry          string
		setUpEnv          setUpEnv
	}
	tests := []struct {
		name              string
		testHomePathEmpty bool
		args              args
		want              Credentials
	}{
		{
			name:              "empty home with correct user prompt",
			testHomePathEmpty: true,
			args: args{
				promptUser:        correctPwdCallback, // user inputs correct credentials
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				setUpEnv:          setEmptyHome,
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "empty config with user prompt",
			args: args{
				promptUser:        correctPwdCallback,
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
	}

	// reset HOME to the original value after tests since they may change it
	defer func() {
		os.Setenv("HOME", homeTempDir)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHomeDir(t)
			// set up HOME
			if tt.testHomePathEmpty {
				os.Unsetenv("HOME")
			} else {
				os.Setenv("HOME", homeTempDir)
			}
			credentialsProvider := creds.NewCredentialsProvider(
				testConfigPath(t),
				creds.WithPromptForCredentials(tt.args.promptUser),
				creds.WithVerifyCredentials(tt.args.verifyCredentials),
			)

			got, err := credentialsProvider(context.Background(), tt.args.registry+"/someorg/someimage:sometag")

			// ASSERT
			if err != nil {
				t.Errorf("%v", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got: %v, want: %v", got, tt.want)
			}
		})
	}
}

// TestCredentialsHomePermissions tests whether the credentials provider
// works in scenarios where HOME has different permissions
func TestCredentialsHomePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip windows perms for this test until windows perms are added")
	}

	if os.Getenv("GITHUB_ACTION") == "" {
		// skip for prow because its running as root
		t.Skip()
	}

	type args struct {
		promptUser        creds.CredentialsCallback
		verifyCredentials creds.VerifyCredentialsCallback
		registry          string
		setUpEnv          setUpEnv
	}
	tests := []struct {
		name              string
		perms             os.FileMode
		args              args
		expPermsDeniedErr bool
		want              Credentials
	}{
		{
			name:  "home with 0000 permissions (no perms)",
			perms: 0000,
			args: args{
				promptUser: pwdCbkThatShallNotBeCalled(t),
				setUpEnv:   setHomeWithPermissions(0000),
			},
			expPermsDeniedErr: true,
		},
		{
			name:  "home with 0333 permissions (write-execute only)",
			perms: 0333,
			args: args{

				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				setUpEnv: all(
					withPopulatedDockerAuthConfig,
					setUpMockHelper("docker-credential-mock", newInMemoryHelper()),
					setHomeWithPermissions(0333)),
			},

			expPermsDeniedErr: false,
			want:              Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name:  "home with 0444 permissions (read-only)",
			perms: 0444,
			args: args{
				promptUser: pwdCbkThatShallNotBeCalled(t),
				setUpEnv:   setHomeWithPermissions(0444)},
			expPermsDeniedErr: true,
		},
		{
			name:  "home with 0555 permissions (read-execute-only)",
			perms: 0555,
			args: args{
				promptUser: pwdCbkThatShallNotBeCalled(t),
				setUpEnv:   setHomeWithPermissions(0555),
			},
			expPermsDeniedErr: true,
		},
		{
			name:  "home with 0666 permissions (read-write-execute)",
			perms: 0666,
			args: args{
				promptUser: pwdCbkThatShallNotBeCalled(t),
				setUpEnv:   setHomeWithPermissions(0666),
			},
			expPermsDeniedErr: true,
		},
		{
			name:  "home with 0777 permissions (full access)",
			perms: 0777,

			args: args{
				promptUser:        pwdCbkThatShallNotBeCalled(t),
				verifyCredentials: correctVerifyCbk,
				registry:          "docker.io",
				setUpEnv: all(
					withPopulatedDockerAuthConfig,
					setUpMockHelper("docker-credential-mock", newInMemoryHelper()),
					setHomeWithPermissions(0777),
				),
			},
			expPermsDeniedErr: false,
			want:              Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
	}

	// return HOME dir into its original state
	defer func() {
		resetHomePermissions(t) //reset home permissions to 0700
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			resetHomePermissions(t) //needs to be reset so that dir can be removed
			resetHomeDir(t)

			if tt.args.setUpEnv != nil {
				tt.args.setUpEnv(t)
			}

			// try to create HOME/.config/func
			_, err := testConfigPathError(t)
			if err != nil { // If error was returned
				if os.IsPermission(err) { // and its a permission error
					if !tt.expPermsDeniedErr { // but it wasnt expected
						t.Fatalf("didnt expect permissions denied error, but got: %s", err)
					}

				} else { // and it wasnt permission error
					t.Fatalf("got unexpected error: %v", err)
				}
			} else { // Else no error was returned
				if tt.expPermsDeniedErr { // but it was expected
					t.Fatal("expected permissions denied error, but got none")
				}
			}

			// if permissions were not denied, try to create Provider
			if !tt.expPermsDeniedErr {

				// try to stat HOME for permissions
				info, err := os.Stat(os.Getenv(testHomeEnvName()))
				if err != nil {
					t.Fatalf("failed to stat HOME: %s", err)
				}

				if info.Mode().Perm() != tt.perms {
					t.Errorf("expected permissions '%v', got '%v'", tt.perms, info.Mode().Perm())
				}
				credentialsProvider := creds.NewCredentialsProvider(
					testConfigPath(t),
					creds.WithPromptForCredentials(tt.args.promptUser),
					creds.WithVerifyCredentials(tt.args.verifyCredentials),
				)

				got, err := credentialsProvider(context.Background(), tt.args.registry+"/someorg/someimage:sometag")
				if err != nil {
					t.Errorf("%v", err)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("got: %v, want: %v", got, tt.want)
				}

			}
		})
	}
}

// ********************** helper functions below **************************** \\

func resetHomeDir(t *testing.T) {
	t.TempDir()
	if err := os.RemoveAll(homeTempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeTempDir, 0700); err != nil {
		t.Fatal(err)
	}
}

// resetHomePermissions resets the HOME perms to 0700 (same as resetHomeDir(t))
func resetHomePermissions(t *testing.T) {
	if err := os.Chmod(homeTempDir, 0700); err != nil {
		t.Fatal(err)
	}
}

type setUpEnv = func(t *testing.T)

func withPopulatedDockerAuthConfig(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dockerConfigDir := filepath.Join(home, ".docker")
	dockerConfigPath := filepath.Join(dockerConfigDir, "config.json")
	err = os.MkdirAll(filepath.Dir(dockerConfigPath), 0700)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dockerConfigDir) })

	configJSON := `{
	"auths": {
		"docker.io": { "auth": "%s" },
		"quay.io": {}
	},
	"credsStore": "mock"
}`
	configJSON = fmt.Sprintf(configJSON, base64.StdEncoding.EncodeToString([]byte(dockerIoUser+":"+dockerIoUserPwd)))

	err = os.WriteFile(dockerConfigPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatal(err)
	}
}

func withPopulatedFuncAuthConfig(t *testing.T) {
	t.Helper()

	var err error

	authConfig := filepath.Join(testConfigPath(t), "auth.json")
	err = os.MkdirAll(filepath.Dir(authConfig), 0700)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(authConfig) })

	authJSON := `{
	"auths": {
		"docker.io": { "auth": "%s" },
		"quay.io":   { "auth": "%s" }
	}
}`
	authJSON = fmt.Sprintf(authJSON,
		base64.StdEncoding.EncodeToString([]byte(dockerIoUser+":"+dockerIoUserPwd)),
		base64.StdEncoding.EncodeToString([]byte(quayIoUser+":"+quayIoUserPwd)))

	err = os.WriteFile(authConfig, []byte(authJSON), 0600)
	if err != nil {
		t.Fatal(err)
	}
}

func pwdCbkThatShallNotBeCalled(t *testing.T) creds.CredentialsCallback {
	t.Helper()
	return func(registry string) (Credentials, error) {
		return Credentials{}, errors.New("this pwd cbk code shall not be called")
	}
}

func pwdCbkFirstWrongThenCorrect(t *testing.T) func(registry string) (Credentials, error) {
	t.Helper()
	var firstInvocation bool
	return func(registry string) (Credentials, error) {
		// registry is in form of registry/repository, need to extract registry only
		registry = strings.Split(registry, "/")[0]
		if registry != "index.docker.io" && registry != "quay.io" {
			return Credentials{}, fmt.Errorf("unexpected registry: %s", registry)
		}
		if firstInvocation {
			firstInvocation = false
			return Credentials{Username: dockerIoUser, Password: "badPwd"}, nil
		}
		return correctPwdCallback(registry)
	}
}

func correctPwdCallback(registry string) (Credentials, error) {
	// registry is in form of registry/repository, need to extract registry only
	registry = strings.Split(registry, "/")[0]
	if registry == "index.docker.io" {
		return Credentials{Username: dockerIoUser, Password: dockerIoUserPwd}, nil
	}
	if registry == "quay.io" {
		return Credentials{Username: quayIoUser, Password: quayIoUserPwd}, nil
	}
	return Credentials{}, errors.New("this cbk don't know the pwd")
}

func correctVerifyCbk(ctx context.Context, image string, credentials Credentials) error {
	username, password := credentials.Username, credentials.Password
	if username == dockerIoUser && password == dockerIoUserPwd && image == "docker.io/someorg/someimage:sometag" {
		return nil
	}
	if username == quayIoUser && password == quayIoUserPwd && image == "quay.io/someorg/someimage:sometag" {
		return nil
	}
	return creds.ErrUnauthorized
}

func testHomeEnvName() string {
	if runtime.GOOS == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}

func testConfigPath(t *testing.T) string {
	t.Helper()
	home := os.Getenv(testHomeEnvName())
	var configPath string
	if home != "" { // if HOME is not set, don't create config dir
		configPath = filepath.Join(home, ".config", "func")
		if err := os.MkdirAll(configPath, os.ModePerm); err != nil {
			t.Fatal(err)
		}
	}
	return configPath
}

// testConfigPathError tries to create a config dir in HOME/.config/func.
// Compared to testConfigPath, this returns the path AND error instead of failing
func testConfigPathError(t *testing.T) (string, error) {
	t.Helper()
	home := os.Getenv(testHomeEnvName())
	configPath := filepath.Join(home, ".config", "func")
	return configPath, os.MkdirAll(configPath, os.ModePerm)
}

func handlerForCredHelper(t *testing.T, credHelper credentials.Helper) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		var err error
		var outBody interface{}

		uri := strings.Trim(request.RequestURI, "/")

		var serverURL string
		if uri == "get" || uri == "erase" {
			data, err := io.ReadAll(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			serverURL = string(data)
			serverURL = strings.Trim(serverURL, "\n\r\t ")
		}

		switch uri {
		case "list":
			var list map[string]string
			list, err = credHelper.List()
			if err == nil {
				outBody = &list
			}
		case "store":
			creds := credentials.Credentials{}
			dec := json.NewDecoder(request.Body)
			err = dec.Decode(&creds)
			if err != nil {
				break
			}
			err = credHelper.Add(&creds)
		case "get":
			var user, secret string
			user, secret, err = credHelper.Get(serverURL)
			if err == nil {
				outBody = &credentials.Credentials{ServerURL: serverURL, Username: user, Secret: secret}
			}
		case "erase":
			err = credHelper.Delete(serverURL)
		default:
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		if err != nil {
			if credentials.IsErrCredentialsNotFound(err) {
				writer.WriteHeader(http.StatusNotFound)
			} else {
				writer.WriteHeader(http.StatusInternalServerError)
				writer.Header().Add("Content-Type", "text/plain")
				fmt.Fprintf(writer, "error: %+v\n", err)
			}
			return
		}

		if outBody != nil {
			var data []byte
			data, err = json.Marshal(outBody)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			writer.Header().Add("Content-Type", "application/json")
			_, err = writer.Write(data)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

}

// Go source code of mock docker-credential-helper implementation.
// Its storage is backed by inMemoryHelper instantiated in test and exposed via HTTP.
const helperGoSrc = `package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	var (
		baseURL = os.Getenv("HELPER_BASE_URL")
		resp *http.Response
		err error
	)
	cmd := os.Args[1]
	switch cmd {
	case "list":
		resp, err = http.Get(baseURL + "/" + cmd)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(os.Stdout, resp.Body)
	case "get", "erase":
		resp, err = http.Post(baseURL+ "/" + cmd, "text/plain", os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(os.Stdout, resp.Body)
	case "store":
		resp, err = http.Post(baseURL+ "/" + cmd, "application/json", os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal(errors.New("unknown cmd: " + cmd))
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatal(errors.New(resp.Status))
	}
	return
}
`

// Creates executable with name determined by the helperName parameter and puts it on $PATH.
//
// The executable behaves like docker credential helper (https://github.com/docker/docker-credential-helpers).
//
// The content of the store presented by the executable is backed by the helper parameter.
func setUpMockHelper(helperName string, helper credentials.Helper) func(t *testing.T) {
	return func(t *testing.T) {

		WithExecutable(t, helperName, helperGoSrc)

		listener, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = listener.Close() })

		baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
		t.Setenv("HELPER_BASE_URL", baseURL)

		server := http.Server{Handler: handlerForCredHelper(t, helper)}
		servErrChan := make(chan error)
		go func() {
			servErrChan <- server.Serve(listener)
		}()

		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			_ = server.Shutdown(ctx)
			e := <-servErrChan
			if !errors.Is(e, http.ErrServerClosed) {
				t.Fatal(e)
			}
		})
	}
}

// combines multiple setUp routines into one setUp routine
func all(fns ...setUpEnv) setUpEnv {
	return func(t *testing.T) {
		t.Helper()
		for _, fn := range fns {
			fn(t)
		}
	}
}

func newInMemoryHelper() *inMemoryHelper {
	return &inMemoryHelper{lock: &sync.Mutex{}, credentials: make(map[string]credentials.Credentials)}
}

type inMemoryHelper struct {
	credentials map[string]credentials.Credentials
	lock        sync.Locker
}

func (i *inMemoryHelper) Add(credentials *credentials.Credentials) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.credentials[credentials.ServerURL] = *credentials

	return nil
}

func (i *inMemoryHelper) Get(serverURL string) (string, string, error) {
	i.lock.Lock()
	defer i.lock.Unlock()

	if result, ok := i.credentials[serverURL]; ok {
		return result.Username, result.Secret, nil
	}

	return "", "", credentials.NewErrCredentialsNotFound()
}

func (i *inMemoryHelper) List() (map[string]string, error) {
	i.lock.Lock()
	defer i.lock.Unlock()

	result := make(map[string]string, len(i.credentials))

	for k, v := range i.credentials {
		result[k] = v.Username
	}

	return result, nil
}

func (i *inMemoryHelper) Delete(serverURL string) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if _, ok := i.credentials[serverURL]; ok {
		delete(i.credentials, serverURL)
		return nil
	}

	return credentials.NewErrCredentialsNotFound()
}

// set home variables to empty values
func setEmptyHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
}

// setHomeWithPermissions sets home dir to specified permissions
func setHomeWithPermissions(perm os.FileMode) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		homeDir := os.Getenv("HOME")

		// if home is empty, nothing to do
		if homeDir == "" {
			t.Fatal("home dir is empty, cant set perms")
		}

		fmt.Printf("setting permissions (%v) on home dir: %s\n", perm, homeDir)
		err := os.Chmod(homeDir, perm)
		if err != nil {
			t.Fatalf("failed to set permissions on home dir: %s", err)
		}
	}
}
