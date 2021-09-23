package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/docker/docker-credential-helpers/client"
)

var ErrCredentialsNotFound = errors.New("credentials not found")

func GetCredentialsFromCredsStore(registry string) (types.DockerAuthConfig, error) {
	result := types.DockerAuthConfig{}

	dirname, err := os.UserHomeDir()
	if err != nil {
		return result, fmt.Errorf("failed to determine home directory: %w", err)
	}

	confFilePath := filepath.Join(dirname, ".docker", "config.json")

	f, err := os.Open(confFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.DockerAuthConfig{}, ErrCredentialsNotFound
		}
		return result, fmt.Errorf("failed to open docker config file: %w", err)
	}
	defer f.Close()

	conf := struct {
		Store string `json:"credsStore"`
	}{}

	decoder := json.NewDecoder(f)

	err = decoder.Decode(&conf)
	if err != nil {
		return result, fmt.Errorf("failed to deserialize docker config file: %w", err)
	}

	if conf.Store == "" {
		return result, fmt.Errorf("no store configured")
	}

	helperName := fmt.Sprintf("docker-credential-%s", conf.Store)
	p := client.NewShellProgramFunc(helperName)

	credentialsMap, err := client.List(p)
	if err != nil {
		return result, fmt.Errorf("failed to list credentials: %w", err)
	}

	for serverUrl := range credentialsMap {
		if registryEquals(serverUrl, registry) {
			creds, err := client.Get(p, serverUrl)
			if err != nil {
				return result, fmt.Errorf("failed to get credentials: %w", err)
			}
			result.Username = creds.Username
			result.Password = creds.Secret
			return result, nil
		}
	}

	return result, fmt.Errorf("failed to get credentials from helper specified in ~/.docker/config.json: %w", ErrCredentialsNotFound)
}

func hostPort(registry string) (host string, port string) {
	host, port = registry, ""
	if !strings.Contains(registry, "://") {
		h, p, err := net.SplitHostPort(registry)

		if err == nil {
			host, port = h, p
			return
		}
		registry = "https://" + registry
	}

	u, err := url.Parse(registry)
	if err != nil {
		panic(err)
	}
	host = u.Hostname()
	port = u.Port()
	return
}

// checks whether registry matches in host and port
// with exception where empty port matches standard ports (80,443)
func registryEquals(regA, regB string) bool {
	h1, p1 := hostPort(regA)
	h2, p2 := hostPort(regB)

	isStdPort := func(p string) bool { return p == "443" || p == "80" }

	portEq := p1 == p2 ||
		(p1 == "" && isStdPort(p2)) ||
		(isStdPort(p1) && p2 == "")

	if h1 == h2 && portEq {
		return true
	}

	if strings.HasSuffix(h1, "docker.io") &&
		strings.HasSuffix(h2, "docker.io") {
		return true
	}

	return false
}
