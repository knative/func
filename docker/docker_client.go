package docker

import (
	"net/http"
	"net/url"
	"os"

	"knative.dev/kn-plugin-func/ssh"

	"github.com/docker/docker/client"
)

func NewDockerClient() (dockerClient client.CommonAPIClient, dockerHost string, err error) {
	dockerHost = os.Getenv("DOCKER_HOST")
	_url, err := url.Parse(dockerHost)
	isSSH := err == nil && _url.Scheme == "ssh"

	if !isSSH {
		dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		return
	}

	credentialsConfig := ssh.Config{
		Identity:           os.Getenv("DOCKER_HOST_SSH_IDENTITY"),
		PassPhrase:         os.Getenv("DOCKER_HOST_SSH_IDENTITY_PASSPHRASE"),
		PasswordCallback:   ssh.NewPasswordCbk(),
		PassPhraseCallback: ssh.NewPassPhraseCbk(),
		HostKeyCallback:    ssh.NewHostKeyCbk(),
	}
	dialContext, dockerHost, err := ssh.NewDialContext(_url, credentialsConfig)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		// No tls
		// No proxy
		Transport: &http.Transport{
			DialContext: dialContext,
		},
	}

	dockerClient, err = client.NewClientWithOpts(
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
		client.WithHost("http://placeholder/"))

	return dockerClient, dockerHost, err
}
