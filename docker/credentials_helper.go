package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/docker/docker-credential-helpers/client"
	"github.com/docker/docker-credential-helpers/credentials"
)

var errCredentialsNotFound = errors.New("credentials not found")
var errNoCredentialHelperConfigured = errors.New("no credential helper configure")

func getCredentialHelperFromConfig(confFilePath string) (string, error) {
	data, err := ioutil.ReadFile(confFilePath)
	if err != nil {
		return "", err
	}

	conf := struct {
		Store string `json:"credsStore"`
	}{}

	err = json.Unmarshal(data, &conf)
	if err != nil {
		return "", err
	}

	return conf.Store, nil
}

func setCredentialHelperToConfig(confFilePath, helper string) error {
	var err error

	configData := make(map[string]interface{})

	if data, err := ioutil.ReadFile(confFilePath); err == nil {
		err = json.Unmarshal(data, &configData)
		if err != nil {
			return err
		}
	}

	configData["credsStore"] = helper

	data, err := json.MarshalIndent(&configData, "", "    ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(confFilePath, data, 0600)
	if err != nil {
		return err
	}

	return nil
}

func getCredentialsByCredentialHelper(confFilePath, registry string) (types.DockerAuthConfig, error) {
	result := types.DockerAuthConfig{}

	helper, err := getCredentialHelperFromConfig(confFilePath)
	if err != nil && !os.IsNotExist(err) {
		return types.DockerAuthConfig{}, fmt.Errorf("failed to get helper from config: %w", err)
	}
	if helper == "" {
		return types.DockerAuthConfig{}, errCredentialsNotFound
	}

	helperName := fmt.Sprintf("docker-credential-%s", helper)
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

	return result, fmt.Errorf("failed to get credentials from helper specified in ~/.docker/config.json: %w", errCredentialsNotFound)
}

func setCredentialsByCredentialHelper(confFilePath, registry, username, secret string) error {
	helper, err := getCredentialHelperFromConfig(confFilePath)

	if helper == "" || os.IsNotExist(err) {
		return errNoCredentialHelperConfigured
	}
	if err != nil {
		return fmt.Errorf("failed to get helper from config: %w", err)
	}

	helperName := fmt.Sprintf("docker-credential-%s", helper)
	p := client.NewShellProgramFunc(helperName)

	return client.Store(p, &credentials.Credentials{ServerURL: registry, Username: username, Secret: secret})
}

func listCredentialHelpers() []string {
	path := os.Getenv("PATH")
	paths := strings.Split(path, string(os.PathListSeparator))

	helpers := make(map[string]bool)
	for _, p := range paths {
		fss, err := ioutil.ReadDir(p)
		if err != nil {
			continue
		}
		for _, fi := range fss {
			if fi.IsDir() {
				continue
			}
			if !strings.HasPrefix(fi.Name(), "docker-credential-") {
				continue
			}
			if runtime.GOOS == "windows" {
				ext := filepath.Ext(fi.Name())
				if ext != "exe" && ext != "bat" {
					continue
				}
			}
			helpers[fi.Name()] = true
		}
	}
	result := make([]string, 0, len(helpers))
	for h := range helpers {
		result = append(result, h)
	}
	return result
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
