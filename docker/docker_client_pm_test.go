//go:build !linux
// +build !linux

package docker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"knative.dev/kn-plugin-func/docker"
	. "knative.dev/kn-plugin-func/testing"
)

func TestNewDockerClientWithPodmanMachine(t *testing.T) {
	defer withCleanHome(t)()

	publicKey, privateKeyPath := prepareKeys(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	sshConf, stopSSH := startSSH(t, publicKey)
	defer stopSSH()
	_ = sshConf

	uri := fmt.Sprintf("ssh://user@%s%s", sshConf.address, sshDockerSocket)
	out := fmt.Sprintf(`[{"Name":"podman-machine-default","URI":%q,"Identity":%q,"Default":true}]`, uri, privateKeyPath)

	goSrc := fmt.Sprintf("package main; import \"fmt\"; func main() { fmt.Println(%q); }", out)

	t.Setenv("DOCKER_HOST", "")
	WithExecutable(t, "podman", goSrc)

	dockerClient, dockerHostInRemote, err := docker.NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	_ = dockerHostInRemote
	if dockerHostInRemote != `unix://`+sshDockerSocket {
		t.Errorf("bad remote DOCKER_HOST: expected %q but got %q", `unix://`+sshDockerSocket, dockerHostInRemote)
	}

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Error(err)
	}
}

func prepareKeys(t *testing.T) (publicKey ssh.PublicKey, privateKeyPath string) {
	var err error

	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err = ssh.NewPublicKey(&pk.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	bs, err := x509.MarshalECPrivateKey(pk)
	if err != nil {
		t.Fatal(err)
	}

	blk := pem.Block{
		Type:    "EC PRIVATE KEY",
		Headers: nil,
		Bytes:   bs,
	}

	tmpDir := t.TempDir()
	privateKeyPath = filepath.Join(tmpDir, "id")

	f, err := os.OpenFile(privateKeyPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = pem.Encode(f, &blk)
	if err != nil {
		t.Fatal(err)
	}

	return
}
