package docker

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/docker/go-connections/tlsconfig"
	mobyClient "github.com/moby/moby/client"
	gossh "golang.org/x/crypto/ssh"

	fnssh "knative.dev/func/pkg/ssh"
)

var ErrNoDocker = errors.New("docker/podman API not available")

// dockerHostInfo holds the results of docker host discovery.
type dockerHostInfo struct {
	dockerHost       string
	dockerHostRemote string
	sshIdentity      string
	hostKeyCallback  fnssh.HostKeyCallback
	isSSH            bool
	isTCP            bool
}

// resolveDockerHost performs docker host discovery.
// It reads DOCKER_HOST env var, checks socket existence, detects podman,
// and determines the docker host for use inside containers.
// If needsPodmanService is true, the caller should spawn a podman service (Linux only).
func resolveDockerHost(defaultHost string) (info *dockerHostInfo, needsPodmanService bool, err error) {
	info = &dockerHostInfo{
		hostKeyCallback: fnssh.NewHostKeyCbk(),
	}

	dockerHost := os.Getenv("DOCKER_HOST")
	dockerHostSSHIdentity := os.Getenv("DOCKER_HOST_SSH_IDENTITY")

	if dockerHost == "" {
		_url, e := url.Parse(defaultHost)
		if e != nil {
			err = e
			return
		}
		_, statErr := os.Stat(_url.Path)
		switch {
		case statErr == nil:
			dockerHost = defaultHost
		case statErr != nil && !os.IsNotExist(statErr):
			err = statErr
			return
		case os.IsNotExist(statErr) && podmanPresent():
			if runtime.GOOS == "linux" {
				needsPodmanService = true
				return
			}
			// on non-Linux: try to use connection to podman machine
			dh, dhid := tryGetPodmanRemoteConn()
			if dh != "" {
				dockerHost, dockerHostSSHIdentity = dh, dhid
				info.hostKeyCallback = fnssh.HostKeyCallback(
					func(string, gossh.PublicKey) error { return nil },
				)
			}
		}
	}

	if dockerHost == "" {
		err = ErrNoDocker
		return
	}

	info.dockerHost = dockerHost
	info.dockerHostRemote = dockerHost
	info.sshIdentity = dockerHostSSHIdentity

	_url, e := url.Parse(dockerHost)
	info.isSSH = e == nil && _url.Scheme == "ssh"
	info.isTCP = e == nil && _url.Scheme == "tcp"
	isNPipe := e == nil && _url.Scheme == "npipe"
	isUnix := e == nil && _url.Scheme == "unix"

	if info.isTCP || isNPipe {
		info.dockerHostRemote = ""
	}

	if isUnix && (runtime.GOOS == "darwin" || strings.HasSuffix(dockerHost, ".docker/desktop/docker.sock")) {
		info.dockerHostRemote = ""
	}

	return
}

// NewClient creates a new docker client.
// reads the DOCKER_HOST envvar but it may or may not return it as dockerHost.
//   - For local connection (unix socket and windows named pipe) it returns the
//     DOCKER_HOST directly.
//   - For ssh connections it reads the DOCKER_HOST from the ssh remote.
//   - For TCP connections it returns "" so it defaults in the remote (note that
//     one should not be use mobyClient.DefaultDockerHost in this situation). This is
//     needed because of TCP+tls connections.
func NewClient(defaultHost string) (mobyClient.APIClient, string, error) {
	info, needsPodman, err := resolveDockerHost(defaultHost)
	if err != nil {
		return nil, "", err
	}

	if needsPodman {
		cli, host, err := newClientWithPodmanService()
		if err != nil {
			return nil, "", err
		}
		return &closeGuardingClient{pimpl: cli}, host, nil
	}

	if !info.isSSH {
		opts := []mobyClient.Opt{mobyClient.FromEnv}
		if info.isTCP {
			if httpClient := newTLSHTTPClient(mobyClient.CheckRedirect); httpClient != nil {
				opts = append(opts, mobyClient.WithHTTPClient(httpClient))
			}
		}
		cli, err := mobyClient.New(opts...)
		if err != nil {
			return nil, "", err
		}
		return &closeGuardingClient{pimpl: cli}, info.dockerHostRemote, nil
	}

	// SSH case
	credentialsConfig := fnssh.Config{
		Identity:           info.sshIdentity,
		PassPhrase:         os.Getenv("DOCKER_HOST_SSH_IDENTITY_PASSPHRASE"),
		PasswordCallback:   fnssh.NewPasswordCbk(),
		PassPhraseCallback: fnssh.NewPassPhraseCbk(),
		HostKeyCallback:    info.hostKeyCallback,
	}
	_url, _ := url.Parse(info.dockerHost)
	contextDialer, dockerHostInRemote, err := fnssh.NewDialContext(_url, credentialsConfig)
	if err != nil {
		return nil, "", err
	}

	httpClient := &http.Client{
		// No tls
		// No proxy
		Transport: &http.Transport{
			DialContext: contextDialer.DialContext,
		},
	}

	cli, err := mobyClient.New(
		mobyClient.WithHTTPClient(httpClient))
	if err != nil {
		return nil, "", err
	}

	cg := &closeGuardingClient{pimpl: cli}
	if closer, ok := contextDialer.(io.Closer); ok {
		cg.cleanUp = func() {
			closer.Close()
		}
	}

	return cg, dockerHostInRemote, nil
}

// newTLSHTTPClient returns an HTTP client configured for TLS if DOCKER_TLS_VERIFY is set.
// Otherwise, nil is returned. The checkRedirect parameter should be
// mobyClient.CheckRedirect or client.CheckRedirect depending on the caller.
func newTLSHTTPClient(checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	tlsVerifyStr, tlsVerifyChanged := os.LookupEnv("DOCKER_TLS_VERIFY")

	if !tlsVerifyChanged {
		return nil
	}

	var tlsOpts []func(*tls.Config)

	tlsVerify := true
	if b, err := strconv.ParseBool(tlsVerifyStr); err == nil {
		tlsVerify = b
	}

	if !tlsVerify {
		tlsOpts = append(tlsOpts, func(t *tls.Config) {
			t.InsecureSkipVerify = true
		})
	}

	dockerCertPath := os.Getenv("DOCKER_CERT_PATH")
	if dockerCertPath == "" {
		dockerCertPath = config.Dir()
	}

	// Set root CA.
	caData, err := os.ReadFile(filepath.Join(dockerCertPath, "ca.pem"))
	if err == nil {
		certPool := x509.NewCertPool()
		if certPool.AppendCertsFromPEM(caData) {
			tlsOpts = append(tlsOpts, func(t *tls.Config) {
				t.RootCAs = certPool
			})
		}
	}

	// Set client certificate.
	certData, certErr := os.ReadFile(filepath.Join(dockerCertPath, "cert.pem"))
	keyData, keyErr := os.ReadFile(filepath.Join(dockerCertPath, "key.pem"))
	if certErr == nil && keyErr == nil {
		cliCert, err := tls.X509KeyPair(certData, keyData)
		if err == nil {
			tlsOpts = append(tlsOpts, func(cfg *tls.Config) {
				cfg.Certificates = []tls.Certificate{cliCert}
			})
		}
	}

	dialer := &net.Dialer{
		KeepAlive: 30 * time.Second,
		Timeout:   30 * time.Second,
	}

	tlsConfig := tlsconfig.ClientDefault(tlsOpts...)

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			DialContext:     dialer.DialContext,
		},
		CheckRedirect: checkRedirect,
	}
}

// tries to get connection to default podman machine
func tryGetPodmanRemoteConn() (uri string, identity string) {
	cmd := exec.Command("podman", "system", "connection", "list", "--format=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", ""
	}
	var connections []struct {
		Name     string
		URI      string
		Identity string
		Default  bool
	}
	err = json.Unmarshal(out, &connections)
	if err != nil {
		return "", ""
	}

	for _, c := range connections {
		if c.Default {
			uri = c.URI
			identity = c.Identity
			break
		}
	}

	return uri, identity
}

func podmanPresent() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}
