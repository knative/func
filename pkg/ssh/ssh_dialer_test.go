package ssh_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	th "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"

	funcssh "knative.dev/func/pkg/ssh"
)

type args struct {
	connStr          string
	credentialConfig funcssh.Config
}
type testParams struct {
	name        string
	args        args
	setUpEnv    setUpEnvFn
	skipOnWin   bool
	skipOnRoot  bool
	CreateError string
	DialError   string
}

func TestCreateDialer(t *testing.T) {

	clientPrivKeyRSA, clientPrivKeyECDSA := generateClientKeys(t)

	withoutSSHAgent(t)
	withCleanHome(t)

	connConfig, err := prepareSSHServer(t, &clientPrivKeyRSA.PublicKey, &clientPrivKeyECDSA.PublicKey)
	th.AssertNil(t, err)

	time.Sleep(time.Second * 1)

	tests := []testParams{
		{
			name: "read password from input",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{PasswordCallback: func() (string, error) {
					return "idkfa", nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "password in url",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "server key is not in known_hosts (the file doesn't exists)",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome),
			CreateError: funcssh.ErrUnknownServerKeyMsg,
		},
		{
			name: "server key is not in known_hosts (the file exists)",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withEmptyKnownHosts),
			CreateError: funcssh.ErrUnknownServerKeyMsg,
		},
		{
			name: "server key is not in known_hosts (the filed doesn't exists) - user force trust",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{HostKeyCallback: func(hostPort string, pubKey ssh.PublicKey) error {
					return nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome),
		},
		{
			name: "server key is not in known_hosts (the file exists) - user force trust",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{HostKeyCallback: func(hostPort string, pubKey ssh.PublicKey) error {
					return nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withEmptyKnownHosts),
		},
		{
			name: "server key does not match the respective key in known_host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withBadKnownHosts(connConfig)),
			CreateError: funcssh.ErrBadServerKeyMsg,
		},
		{
			name: "key from identity parameter",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with need to read passphrase",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{PassPhraseCallback: func() (string, error) {
					return "nbusr123", nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(clientPrivKeyRSA, "id_rsa", "nbusr123"), withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with explicitly set passphrase",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{PassPhrase: "nbusr123"},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(clientPrivKeyECDSA, "id_ecdsa", "nbusr123"), withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with no passphrase",
			args: args{connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(clientPrivKeyECDSA, "id_ecdsa", ""), withKnowHosts(connConfig)),
		},
		{
			name: "key from ssh-agent",
			args: args{connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withGoodSSHAgent(clientPrivKeyRSA, clientPrivKeyECDSA), withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "password in url with IPv6",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@[%s]:%d/home/testuser/test.sock",
				connConfig.hostIPv6,
				connConfig.portIPv6,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "broken known host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withBrokenKnownHosts),
			CreateError: "invalid entry in known_hosts",
		},
		{
			name: "inaccessible known host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withInaccessibleKnownHosts),
			skipOnWin:   true,
			skipOnRoot:  true,
			CreateError: "permission denied",
		},
		{
			name: "failing pass phrase cbk",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{PassPhraseCallback: func() (string, error) {
					return "", errors.New("test_error_msg")
				}},
			},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withKey(clientPrivKeyRSA, "id_rsa", "nbusr123"), withKnowHosts(connConfig)),
			CreateError: "test_error_msg",
		},
		{
			name: "with broken key at default location",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withGibberishKey("id_dsa"), withKnowHosts(connConfig)),
			CreateError: "failed to parse private key",
		},
		{
			name: "with broken key explicit",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: gibberishKey(t)},
			},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to parse private key",
		},
		{
			name: "with inaccessible key",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withInaccessibleKey("id_rsa"), withKnowHosts(connConfig)),
			skipOnWin:   true,
			skipOnRoot:  true,
			CreateError: "failed to read key file",
		},
		{
			name: "socket doesn't exist in remote",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{PasswordCallback: func() (string, error) {
					return "idkfa", nil
				}},
			},
			setUpEnv:  all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			DialError: "failed to dial unix socket in the remote",
		},
		{
			name: "ssh agent non-existent socket",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
			},
			setUpEnv:    all(withBadSSHAgentSocket, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to connect to ssh-agent's socket",
		},
		{
			name: "bad ssh agent",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
			},
			setUpEnv:    all(withBadSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to get signers from ssh-agent",
		},
		{
			name: "use docker host from remote unix",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("unix:///home/testuser/test.sock", connConfig)),
		},
		{
			name: "use docker host from remote tcp",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("tcp://localhost:1234", connConfig)),
		},
		{
			name: "use docker host from remote fd",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("fd://localhost:1234", connConfig)),
		},
		{
			name: "windows without docker system dial-stdio",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withEmulatingWindows(connConfig)),
			CreateError: "cannot use dial-stdio",
		},
		{
			name: "windows with system dial-stdio",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: funcssh.Config{Identity: tempKey(t, clientPrivKeyECDSA, "")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withEmulatingWindows(connConfig), withKnowHosts(connConfig),
				withEmulatedDockerSystemDialStdio(connConfig), withFixedUpSSHCLI),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.args.connStr)
			th.AssertNil(t, err)

			if net.ParseIP(u.Hostname()).To4() == nil && connConfig.hostIPv6 == "" {
				t.Skip("skipping ipv6 test since test environment doesn't support ipv6 connection")
			}

			if tt.skipOnWin && runtime.GOOS == "windows" {
				t.Skip("skipping this test on windows")
			}

			if tt.skipOnRoot && os.Geteuid() == 0 {
				t.Skip("skipping this test when running as a root")
			}

			tt.setUpEnv(t)

			dialContext, _, err := funcssh.NewDialContext(u, tt.args.credentialConfig)

			if tt.CreateError == "" {
				th.AssertEq(t, err, nil)
			} else {
				// I wish I could use errors.Is(),
				// however foreign code is not wrapping errors thoroughly
				if err != nil {
					th.AssertContains(t, err.Error(), tt.CreateError)
				} else {
					t.Error("expected error but got nil")
				}
			}
			if err != nil {
				return
			}

			transport := http.Transport{DialContext: dialContext.DialContext}
			httpClient := http.Client{Transport: &transport}
			defer httpClient.CloseIdleConnections()
			resp, err := httpClient.Get("http://docker/")
			if tt.DialError == "" {
				th.AssertNil(t, err)
			} else {
				// I wish I could use errors.Is(),
				// however foreign code is not wrapping errors thoroughly
				if err != nil {
					th.AssertContains(t, err.Error(), tt.CreateError)
				} else {
					t.Error("expected error but got nil")
				}
			}
			if err != nil {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			th.AssertTrue(t, err == nil)
			if err != nil {
				return
			}
			th.AssertEq(t, string(b), "OK")
		})
	}
}

// function that prepares testing environment and returns clean up function
// this should be used in conjunction with defer: `defer fn()()`
// e.g. sets environment variables or starts mock up services
// it returns clean up procedure that restores old values of environment variables
// or shuts down mock up services
type setUpEnvFn func(t *testing.T)

// combines multiple setUp routines into one setUp routine
func all(fns ...setUpEnvFn) setUpEnvFn {
	return func(t *testing.T) {
		//t.Helper()

		for _, fn := range fns {
			fn(t)
		}
	}
}

// puts private key to $HOME/.ssh/{keyName}
func withKey(key any, keyName, passphrase string) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()

		home, err := os.UserHomeDir()
		th.AssertNil(t, err)

		err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		th.AssertNil(t, err)

		keyDest := filepath.Join(home, ".ssh", keyName)

		marshallKey(t, key, keyDest, passphrase)

		t.Cleanup(func() {
			_ = os.Remove(keyDest)
		})
	}
}

func gibberishKey(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "id")
	err := os.WriteFile(p, []byte("definetelynotakey"), 0600)
	th.AssertNil(t, err)
	return p
}

func withGibberishKey(keyName string) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()

		home, err := os.UserHomeDir()
		th.AssertNil(t, err)

		err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		th.AssertNil(t, err)

		keyDest := filepath.Join(home, ".ssh", keyName)
		err = os.WriteFile(keyDest, []byte("definetelynotakey"), 0600)
		th.AssertNil(t, err)
	}
}

// this function marshals key to temporary file and returns its path
func tempKey(t *testing.T, key any, passphrase string) string {
	p := filepath.Join(t.TempDir(), "id")
	marshallKey(t, key, p, passphrase)
	return p
}

func marshallKey(t *testing.T, key any, destPath, passphrase string) {
	var (
		err     error
		raw     []byte
		pemType string
	)

	if k, ok := key.(*rsa.PrivateKey); ok {
		pemType = "RSA PRIVATE KEY"
		raw = x509.MarshalPKCS1PrivateKey(k)
	} else if k, ok := key.(*ecdsa.PrivateKey); ok {
		pemType = "EC PRIVATE KEY"
		raw, err = x509.MarshalECPrivateKey(k)
		th.AssertNil(t, err)
	} else {
		panic("unsupported key type")
	}

	blk := &pem.Block{
		Type:  pemType,
		Bytes: raw,
	}

	if passphrase != "" {
		//nolint:staticcheck
		blk, err = x509.EncryptPEMBlock(rand.New(rand.NewSource(time.Now().UnixNano())), blk.Type, blk.Bytes, []byte(passphrase), x509.PEMCipherAES256)
		th.AssertNil(t, err)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0600)
	th.AssertNil(t, err)
	defer f.Close()

	err = pem.Encode(f, blk)
	th.AssertNil(t, err)
	_ = f.Close()

	fixupPrivateKeyMod(destPath)
}

// withInaccessibleKey creates inaccessible key of give type (specified by keyName)
func withInaccessibleKey(keyName string) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()
		var err error

		home, err := os.UserHomeDir()
		th.AssertNil(t, err)

		err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		th.AssertNil(t, err)

		keyDest := filepath.Join(home, ".ssh", keyName)
		f, err := os.OpenFile(keyDest, os.O_CREATE|os.O_WRONLY, 0000)
		th.AssertNil(t, err)
		f.Close()

		t.Cleanup(func() {
			_ = os.Remove(keyDest)
		})
	}
}

// sets clean temporary $HOME for test
// this prevents interaction with actual user home which may contain .ssh/
func withCleanHome(t *testing.T) {
	t.Helper()
	homeName := "HOME"
	if runtime.GOOS == "windows" {
		homeName = "USERPROFILE"
	}
	tempHome := t.TempDir()
	t.Setenv(homeName, tempHome)
}

// withKnowHosts creates $HOME/.ssh/known_hosts with correct entries
func withKnowHosts(connConfig *SSHServer) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()

		knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

		err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
		th.AssertNil(t, err)

		_, err = os.Stat(knownHosts)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatal("known_hosts already exists")
		}

		f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
		th.AssertNil(t, err)
		defer f.Close()

		// generate known_hosts
		for _, privKey := range connConfig.serverKeys {
			pubKey := publicKey(privKey)
			k, err := ssh.NewPublicKey(pubKey)
			if err != nil {
				t.Fatal(err)
			}
			bs := ssh.MarshalAuthorizedKey(k)

			fmt.Fprintf(f, "%s %s", connConfig.hostIPv4, string(bs))
			fmt.Fprintf(f, "[%s]:%d %s", connConfig.hostIPv4, connConfig.portIPv4, string(bs))

			if connConfig.hostIPv6 != "" {
				fmt.Fprintf(f, "%s %s", connConfig.hostIPv6, string(bs))
				fmt.Fprintf(f, "[%s]:%d %s", connConfig.hostIPv6, connConfig.portIPv6, string(bs))
			}
		}
		t.Cleanup(func() {
			_ = os.Remove(knownHosts)
		})
	}
}

func publicKey(privKey any) any {
	switch privKey := privKey.(type) {
	case *rsa.PrivateKey:
		return &privKey.PublicKey
	case *ecdsa.PrivateKey:
		return &privKey.PublicKey
	default:
		panic("unsupported key type")
	}
}

// withBadKnownHosts creates $HOME/.ssh/known_hosts with incorrect entries
func withBadKnownHosts(connConfig *SSHServer) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()

		knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

		err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
		th.AssertNil(t, err)

		_, err = os.Stat(knownHosts)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatal("known_hosts already exists")
		}

		f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
		th.AssertNil(t, err)
		defer f.Close()

		knownHostTemplate := `{{range $host := .}}{{$host}} ssh-dss AAAAB3NzaC1kc3MAAACBAKH4ufS3ABVb780oTgEL1eu+pI1p6YOq/1KJn5s3zm+L3cXXq76r5OM/roGEYrXWUDGRtfVpzYTAKoMWuqcVc0AZ2zOdYkoy1fSjJ3MqDGF53QEO3TXIUt3gUzmLOewwmZWle0RgMa9GHccv7XVVIZB36RR68ZEUswLaTnlVhXQ1AAAAFQCl4t/LnY7kuUI+tL2qT2XmxmiyqwAAAIB72XaO+LfyIiqBOaTkQf+5rvH1i6y6LDO1QD9pzGWUYw3y03AEveHJMjW0EjnYBKJjK39wcZNTieRyU54lhH/HWeWABn9NcQ3duEf1WSO/s7SPsFO2R6quqVSsStkqf2Yfdy4fl24mH41olwtNA6ft5nkVfkqrIa51si4jU8fBVAAAAIB8SSvyYBcyMGLUlQjzQqhhhAHer9x/1YbknVz+y5PHJLLjHjMC4ZRfLgNEojvMKQW46Te9Pwnudcwv19ho4F+kkCOfss7xjyH70gQm6Sj76DxClmnnPoSRq3qEAOMy5Oh+7vyzxm68KHqd/aOmUaiT1LgqgViS9+kNdCoVMGAMOg== mvasek@bellatrix
{{$host}} ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBKPrqGp4c5ZstymDqXOxPsIEH6e6a4Pi8qcTRUkbyQllWjyQVx0A/o4yA8cd222x3t9gsiGa+mNgCYkyFehH0nKO7gk057jNmALc9xhbj25EdmREjdex+yUrmxdxcG9mtQ==
{{$host}} ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOKymJNQszrxetVffPZRfZGKWK786r0mNcg/Wah4+2wn mvasek@bellatrix
{{$host}} ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC/1/OCwec2Gyv5goNYYvos4iOA+a0NolOGsZA/93jmSArPY1zZS1UWeJ6dDTmxGoL/e7jm9lM6NJY7a/zM0C/GqCNRGR/aCUHBJTIgGtH+79FDKO/LWY6ClGY7Lw8qNgZpugbBw3N3HqTtyb2lELhFLT0FEb+le4WUbryooLK2zsz6DnqV4JvTYyyHcanS0h68iSXC7XbkZchvL99l5LT0gD1oDteBPKKFdNOwIjpMkk/IrbFM24xoNkaTDXN87EpQPQzYDfsoGymprc5OZZ8kzrtErQR+yfuunHfzzqDHWi7ga5pbgkuxNt10djWgCfBRsy07FTEgV0JirS0TCfwTBbqRzdjf3dgi8AP+WtkW3mcv4a1XYeqoBo2o9TbfyiA9kERs79UBN0mCe3KNX3Ns0PvutsRLaHmdJ49eaKWkJ6GgL37aqSlIwTixz2xY3eoDSkqHoZpx6Q1MdpSIl5gGVzlaobM/PNM1jqVdyUj+xpjHyiXwHQMKc3eJna7s8Jc= mvasek@bellatrix
{{end}}`

		tmpl := template.New(knownHostTemplate)
		tmpl, err = tmpl.Parse(knownHostTemplate)
		th.AssertNil(t, err)

		hosts := make([]string, 0, 4)
		hosts = append(hosts, connConfig.hostIPv4, fmt.Sprintf("[%s]:%d", connConfig.hostIPv4, connConfig.portIPv4))
		if connConfig.hostIPv6 != "" {
			hosts = append(hosts, connConfig.hostIPv6, fmt.Sprintf("[%s]:%d", connConfig.hostIPv6, connConfig.portIPv4))
		}

		err = tmpl.Execute(f, hosts)
		th.AssertNil(t, err)

		t.Cleanup(func() {
			_ = os.Remove(knownHosts)
		})
	}
}

// withBrokenKnownHosts creates broken $HOME/.ssh/known_hosts
func withBrokenKnownHosts(t *testing.T) {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	th.AssertNil(t, err)

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
	th.AssertNil(t, err)
	defer f.Close()

	_, err = f.WriteString("somegarbage\nsome rubish\n stuff\tqwerty")
	th.AssertNil(t, err)

	t.Cleanup(func() {
		os.Remove(knownHosts)
	})
}

// withInaccessibleKnownHosts creates inaccessible $HOME/.ssh/known_hosts
func withInaccessibleKnownHosts(t *testing.T) {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	th.AssertNil(t, err)

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0000)
	th.AssertNil(t, err)
	defer f.Close()

	t.Cleanup(func() {
		_ = os.Remove(knownHosts)
	})
}

// withEmptyKnownHosts creates empty $HOME/.ssh/known_hosts
func withEmptyKnownHosts(t *testing.T) {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	th.AssertNil(t, err)

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	err = os.WriteFile(knownHosts, []byte{}, 0644)
	th.AssertNil(t, err)

	t.Cleanup(func() {
		_ = os.Remove(knownHosts)
	})
}

// withoutSSHAgent unsets the SSH_AUTH_SOCK environment variable so ssh-agent is not used by test
func withoutSSHAgent(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_AUTH_SOCK", "")
}

// withBadSSHAgentSocket sets the SSH_AUTH_SOCK environment variable to non-existing file
func withBadSSHAgentSocket(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_AUTH_SOCK", "/does/not/exists.sock")
}

// withGoodSSHAgent starts serving ssh-agent on temporary unix socket.
// It sets the SSH_AUTH_SOCK environment variable to the temporary socket.
// The agent will return correct keys for the testing ssh server.
func withGoodSSHAgent(keys ...any) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()
		withSSHAgent(t, signerAgent{keys})
	}
}

// withBadSSHAgent starts serving ssh-agent on temporary unix socket.
// It sets the SSH_AUTH_SOCK environment variable to the temporary socket.
// The agent will return incorrect keys for the testing ssh server.
func withBadSSHAgent(t *testing.T) {
	withSSHAgent(t, badAgent{})
}

func withSSHAgent(t *testing.T, ag agent.Agent) {
	var err error
	t.Helper()

	var tmpDirForSocket string
	var agentSocketPath string
	if runtime.GOOS == "windows" {
		agentSocketPath = `\\.\pipe\openssh-ssh-agent-test`
	} else {
		tmpDirForSocket, err = os.MkdirTemp("", "forAuthSock")
		th.AssertNil(t, err)

		agentSocketPath = filepath.Join(tmpDirForSocket, "agent.sock")
	}

	unixListener, err := listen(agentSocketPath)
	th.AssertNil(t, err)

	os.Setenv("SSH_AUTH_SOCK", agentSocketPath)

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	go func() {
		for {
			conn, err := unixListener.Accept()
			if err != nil {
				errChan <- err

				return
			}

			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				go func() {
					<-ctx.Done()
					conn.Close()
				}()
				err := agent.ServeAgent(ag, conn)
				if err != nil {
					if !isErrClosed(err) {
						fmt.Fprintf(os.Stderr, "agent.ServeAgent() failed: %v\n", err)
					}
				}
			}(conn)
		}
	}()

	t.Cleanup(func() {
		os.Unsetenv("SSH_AUTH_SOCK")

		err := unixListener.Close()
		th.AssertNil(t, err)

		err = <-errChan

		if !isErrClosed(err) {
			t.Fatal(err)
		}
		cancel()
		wg.Wait()
		if tmpDirForSocket != "" {
			os.RemoveAll(tmpDirForSocket)
		}
	})
}

type signerAgent struct {
	keys []any
}

func (a signerAgent) List() ([]*agent.Key, error) {
	result := make([]*agent.Key, 0, len(a.keys))
	for _, key := range a.keys {
		signer, err := ssh.NewSignerFromKey(key)
		if err != nil {
			return nil, err
		}
		result = append(result, &agent.Key{
			Format: signer.PublicKey().Type(),
			Blob:   signer.PublicKey().Marshal(),
		})
	}
	return result, nil
}

func (a signerAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	for _, k := range a.keys {
		signer, err := ssh.NewSignerFromKey(k)
		if err != nil {
			return nil, err
		}
		if signer.PublicKey().Type() == key.Type() &&
			bytes.Equal(signer.PublicKey().Marshal(), key.Marshal()) {
			return signer.Sign(rand.New(rand.NewSource(time.Now().UnixNano())), data)
		}
	}
	return nil, errors.New("key not found")
}

func (a signerAgent) Add(key agent.AddedKey) error {
	panic("implement me")
}

func (a signerAgent) Remove(key ssh.PublicKey) error {
	panic("implement me")
}

func (a signerAgent) RemoveAll() error {
	panic("implement me")
}

func (a signerAgent) Lock(passphrase []byte) error {
	panic("implement me")
}

func (a signerAgent) Unlock(passphrase []byte) error {
	panic("implement me")
}

func (a signerAgent) Signers() ([]ssh.Signer, error) {
	panic("implement me")
}

var errBadAgent = errors.New("bad agent error")

type badAgent struct{}

func (b badAgent) List() ([]*agent.Key, error) {
	return nil, errBadAgent
}

func (b badAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return nil, errBadAgent
}

func (b badAgent) Add(key agent.AddedKey) error {
	return errBadAgent
}

func (b badAgent) Remove(key ssh.PublicKey) error {
	return errBadAgent
}

func (b badAgent) RemoveAll() error {
	return errBadAgent
}

func (b badAgent) Lock(passphrase []byte) error {
	return errBadAgent
}

func (b badAgent) Unlock(passphrase []byte) error {
	return errBadAgent
}

func (b badAgent) Signers() ([]ssh.Signer, error) {
	return nil, errBadAgent
}

// openSSH CLI doesn't take the HOME/USERPROFILE environment variable into account.
// It gets user home in different way (e.g. reading /etc/passwd).
// This means tests cannot mock home dir just by setting environment variable.
// withFixedUpSSHCLI works around the problem, it forces usage of known_hosts from HOME/USERPROFILE.
func withFixedUpSSHCLI(t *testing.T) {
	t.Helper()

	sshAbsPath, err := exec.LookPath("ssh")
	th.AssertNil(t, err)

	sshScript := `#!/bin/sh
SSH_BIN -o PasswordAuthentication=no -o ConnectTimeout=3 -o UserKnownHostsFile="$HOME/.ssh/known_hosts" $@
`
	if runtime.GOOS == "windows" {
		sshScript = `@echo off
"SSH_BIN" -o PasswordAuthentication=no -o ConnectTimeout=3 -o UserKnownHostsFile=%USERPROFILE%\.ssh\known_hosts %*
`
	}
	sshScript = strings.ReplaceAll(sshScript, "SSH_BIN", sshAbsPath)

	home, err := os.UserHomeDir()
	th.AssertNil(t, err)

	homeBin := filepath.Join(home, "bin")
	err = os.MkdirAll(homeBin, 0700)
	th.AssertNil(t, err)

	sshScriptName := "ssh"
	if runtime.GOOS == "windows" {
		sshScriptName = "ssh.bat"
	}

	sshScriptFullPath := filepath.Join(homeBin, sshScriptName)
	err = os.WriteFile(sshScriptFullPath, []byte(sshScript), 0700)
	th.AssertNil(t, err)

	t.Setenv("PATH", homeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		os.RemoveAll(homeBin)
	})
}

// withEmulatedDockerSystemDialStdio makes `docker system dial-stdio` viable in the testing ssh server.
// It does so by appending definition of shell function named `docker` into .bashrc .
func withEmulatedDockerSystemDialStdio(sshServer *SSHServer) setUpEnvFn {
	return func(t *testing.T) {
		t.Helper()

		oldHasDialStdio := sshServer.HasDialStdio()
		sshServer.SetHasDialStdio(true)
		t.Cleanup(func() {
			sshServer.SetHasDialStdio(oldHasDialStdio)
		})
	}
}

// withEmulatingWindows makes changes to the testing ssh server such that
// the server appears to be Windows server for simple check done calling the `systeminfo` command
func withEmulatingWindows(sshServer *SSHServer) setUpEnvFn {
	return func(t *testing.T) {
		oldIsWindows := sshServer.IsWindows()
		sshServer.SetIsWindows(true)
		t.Cleanup(func() {
			sshServer.SetIsWindows(oldIsWindows)
		})
	}
}

// withRemoteDockerHost makes changes to the testing ssh server such that
// the DOCKER_HOST environment is set to host parameter
func withRemoteDockerHost(host string, sshServer *SSHServer) setUpEnvFn {
	return func(t *testing.T) {
		oldHost := sshServer.GetDockerHostEnvVar()
		sshServer.SetDockerHostEnvVar(host)
		t.Cleanup(func() {
			sshServer.SetDockerHostEnvVar(oldHost)
		})
	}
}

func generateClientKeys(t *testing.T) (privKeyRSA *rsa.PrivateKey, privKeyECDSA *ecdsa.PrivateKey) {
	var err error

	privKeyRSA, err = rsa.GenerateKey(rand.New(rand.NewSource(time.Now().UnixNano())), 2048)
	if err != nil {
		t.Fatal(err)
	}

	privKeyECDSA, err = ecdsa.GenerateKey(elliptic.P384(), rand.New(rand.NewSource(time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}

	return privKeyRSA, privKeyECDSA
}
