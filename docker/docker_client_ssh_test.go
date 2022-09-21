package docker_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"
	"knative.dev/kn-plugin-func/docker"
)

func TestNewDockerClientWithSSH(t *testing.T) {
	defer withCleanHome(t)()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	sshConf, stopSSH := startSSH(t)
	defer stopSSH()

	defer withKnowHosts(t, sshConf.address, sshConf.pubHostKey)()

	t.Setenv("DOCKER_HOST", fmt.Sprintf("ssh://user:pwd@%s", sshConf.address))

	dockerClient, dockerHostInRemote, err := docker.NewClient(client.DefaultDockerHost)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	if dockerHostInRemote != `unix://`+sshDockerSocket {
		t.Errorf("bad remote DOCKER_HOST: expected %q but got %q", `unix://`+sshDockerSocket, dockerHostInRemote)
	}

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Error(err)
	}
}

const sshDockerSocket = "/some/path/docker.sock"

type sshConfig struct {
	address    string
	pubHostKey ssh.PublicKey
}

// emulates remote machine with docker unix socket at "/some/path/docker.sock"
func startSSH(t *testing.T, authorizedKeys ...ssh.PublicKey) (settings sshConfig, stopSSH func()) {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	httpServerErrChan := make(chan error, 1)
	pollingLoopErr := make(chan error, 1)

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "pwd" {
				return nil, errors.New("bad pwd")
			}
			return &ssh.Permissions{}, nil
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			for _, authKey := range authorizedKeys {
				if bytes.Equal(authKey.Marshal(), key.Marshal()) {
					return &ssh.Permissions{}, nil
				}
			}
			return nil, fmt.Errorf("unknown public key")
		},
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Error(err)
	}
	hostKey, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Error(err)
	}
	config.AddHostKey(hostKey)
	settings.pubHostKey = hostKey.PublicKey()

	sshTCPListener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	dockerDaemonServer := http.Server{}
	stopSSH = func() {
		var err error
		cancel()

		err = sshTCPListener.Close()
		if err != nil {
			t.Error(err)
		}
		err = <-pollingLoopErr
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		err = dockerDaemonServer.Shutdown(ctx)
		if err != nil {
			t.Error(err)
		}
		err = <-httpServerErrChan
		if err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Error(err)
		}

	}

	settings.address = sshTCPListener.Addr().String()

	t.Logf("Listening on %s", sshTCPListener.Addr())

	// mimics /_ping endpoint
	dockerDaemonServer.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Content-Type", "text/plain")
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("OK"))
	})

	// listener that emulates unix socket in remote accessed via SSH
	dockerDaemonListener := listener{make(chan io.ReadWriteCloser, 128)}

	go func() {
		httpServerErrChan <- dockerDaemonServer.Serve(dockerDaemonListener)
	}()

	handleChannel := func(newChannel ssh.NewChannel) {
		switch newChannel.ChannelType() {
		case "session":
			handleSession(t, newChannel)
		case "direct-streamlocal@openssh.com":
			handleTunnel(t, newChannel, dockerDaemonListener)
		default:
			err = newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("type of channel %q is not supported", newChannel.ChannelType()))
			if err != nil {
				t.Error(err)
			}
		}
	}

	handleChannels := func(newChannels <-chan ssh.NewChannel) {
		for newChannel := range newChannels {
			go handleChannel(newChannel)
		}
	}

	go func() {
		for {
			tcpConn, err := sshTCPListener.Accept()
			if err != nil {
				pollingLoopErr <- err
				return
			}

			sshConn, newChannels, reqs, err := ssh.NewServerConn(tcpConn, config)
			if err != nil {
				pollingLoopErr <- err
				return
			}
			go func() {
				<-ctx.Done()
				err = sshConn.Close()
				if err != nil && !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
			}()

			go ssh.DiscardRequests(reqs)

			go handleChannels(newChannels)
		}
	}()

	return
}

func handleSession(t *testing.T, newChannel ssh.NewChannel) {
	ch, reqs, err := newChannel.Accept()
	if err != nil {
		t.Error(err)
	}
	go func() {
		defer func() {
			_ = ch.Close()
		}()
		for req := range reqs {
			if req.Type == "exec" {
				err = req.Reply(true, nil)
				if err != nil {
					t.Error(err)
				}
				data := struct {
					Command string
				}{}
				err = ssh.Unmarshal(req.Payload, &data)
				if err != nil {
					t.Error(err)
				}
				var ret uint32
				switch {
				case data.Command == "set":
					ret = 0
					_, _ = fmt.Fprintf(ch, "DOCKER_HOST=unix://%s\n", sshDockerSocket)
				default:
					_, _ = fmt.Fprintf(ch.Stderr(), "unknown command: %q\n", data.Command)
					ret = 127
				}
				msg := []byte{0, 0, 0, 0}
				binary.BigEndian.PutUint32(msg, ret)
				_, err = ch.SendRequest("exit-status", false, msg)
				if err != nil {
					t.Error(err)
				}

				return
			}
		}
	}()
}

func handleTunnel(t *testing.T, newChannel ssh.NewChannel, dockerDaemonListener listener) {
	var err error
	extraData := newChannel.ExtraData()
	data := struct {
		SocketPath string
		Reserved0  string
		Reserved1  uint32
	}{}

	err = ssh.Unmarshal(extraData, &data)
	if err != nil {
		t.Error(err)
	}

	if data.SocketPath != sshDockerSocket {
		err = newChannel.Reject(ssh.ConnectionFailed, fmt.Sprintf("bad socket: %q", data.SocketPath))
		if err != nil {
			t.Error(err)
		}
		return
	}

	ch, reqs, err := newChannel.Accept()
	if err != nil {
		t.Error(err)
	}
	select {
	case dockerDaemonListener.connections <- ch:
	default:
		err = ch.Close()
		if err != nil {
			t.Error(err)
		}
		return
	}

	ssh.DiscardRequests(reqs)
}

type listener struct {
	connections chan io.ReadWriteCloser
}

type channelConnection struct {
	ch io.ReadWriteCloser
}

func (c channelConnection) Read(b []byte) (n int, err error) {
	return c.ch.Read(b)
}

func (c channelConnection) Write(b []byte) (n int, err error) {
	return c.ch.Write(b)
}

func (c channelConnection) Close() error {
	return c.ch.Close()
}

func (c channelConnection) LocalAddr() net.Addr {
	return &net.UnixAddr{Name: sshDockerSocket, Net: "unix"}
}

func (c channelConnection) RemoteAddr() net.Addr {
	return &net.UnixAddr{Name: "@", Net: "unix"}
}

func (c channelConnection) SetDeadline(t time.Time) error { return nil }

func (c channelConnection) SetReadDeadline(t time.Time) error { return nil }

func (c channelConnection) SetWriteDeadline(t time.Time) error { return nil }

func (l listener) Accept() (net.Conn, error) {
	rwc, ok := <-l.connections
	if !ok {
		return nil, errors.New("listener closed")
	}
	return channelConnection{rwc}, nil
}

func (l listener) Close() error {
	close(l.connections)
	return nil
}

func (l listener) Addr() net.Addr {
	return &net.UnixAddr{Name: sshDockerSocket, Net: "unix"}
}

// sets clean temporary $HOME for test
// this prevents interaction with actual user home which may contain .ssh/
func withCleanHome(t *testing.T) func() {
	t.Helper()
	homeName := "HOME"
	if runtime.GOOS == "windows" {
		homeName = "USERPROFILE"
	}
	tmpDir, err := ioutil.TempDir("", "tmpHome")
	if err != nil {
		t.Fatal(err)
	}
	oldHome, hadHome := os.LookupEnv(homeName)
	os.Setenv(homeName, tmpDir)

	return func() {
		if hadHome {
			os.Setenv(homeName, oldHome)
		} else {
			os.Unsetenv(homeName)
		}
		os.RemoveAll(tmpDir)
	}
}

// withKnowHosts creates $HOME/.ssh/known_hosts that trust the host
func withKnowHosts(t *testing.T, host string, pubKey ssh.PublicKey) func() {
	t.Helper()

	var err error
	var home string

	home, err = os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(home, ".ssh", "known_hosts")

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	if err != nil {
		t.Fatal(err)
	}

	knownHostFile, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer knownHostFile.Close()

	fmt.Fprintf(knownHostFile, "%s %s\n", host, string(ssh.MarshalAuthorizedKey(pubKey)))

	return func() {
		os.Remove(knownHosts)
	}
}
