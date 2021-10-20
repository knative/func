//go:build e2e || e2elc
// +build e2e e2elc

package e2e

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMain(t *testing.M) {

	if GetRegistry() == defaultRegistry {
		err := patchOrCreateDockerConfigFile()
		if err != nil {
			panic(err.Error())
		}
	}
	t.Run()
}

//
// Here is a trick to avoid calling docker or podman at e2e tests.
// Let's check for default registry credentials in one of the auth sources
// In case it is not present let's create it.
//
func patchOrCreateDockerConfigFile() error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable retrieve user home dir to verify default container authentication. " + err.Error())
	}
	dockerConfigFile := filepath.Join(userHome, ".docker", "config.json")
	_, err = os.Stat(dockerConfigFile)
	if err != nil && os.IsNotExist(err) {
		log.Println("Creating ./docker/config.json file with default registry authentication.")
		err = createConfigAuth(dockerConfigFile, "")
	} else {
		// Read and update it
		err = updateConfigAuth(dockerConfigFile)
	}
	return err
}

func createConfigAuth(dockerConfigFile string, content string) error {
	f, err := os.Create(dockerConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()
	if content == "" {
		content = `{
	"auths": {
		"localhost:5000": {
			"auth": "dXNlcjpwYXNzd29yZA=="
		}
	}
}
`
	}
	_, err = f.WriteString(content)
	if err != nil {
		return fmt.Errorf("unable to create .docker/config.json file" + err.Error())
	}
	return nil
}

func updateConfigAuth(dockerConfigFile string) error {

	bcontent, err := ioutil.ReadFile(dockerConfigFile)
	if err != nil {
		return err
	}
	content := string(bcontent)
	if !strings.Contains(content, strings.Split(defaultRegistry, "/")[0]) {
		// default registry is not present on .docker/config.json, so let's add it
		log.Println("Updating ./docker/config.json file with default registry authentication.")
		exp := regexp.MustCompile(`"auths"[\s]*?[:][\s]*?{`)
		newContent := exp.ReplaceAll(bcontent, []byte(`"auths": {
		"localhost:5000": {
			"auth": "dXNlcjpwYXNzd29yZA=="
		},`))

		// Replace file content
		_ = os.Rename(dockerConfigFile, dockerConfigFile+".e2e")
		err := createConfigAuth(dockerConfigFile, string(newContent))
		if err != nil {
			// rollback config file
			_ = os.Rename(dockerConfigFile+".e2e", dockerConfigFile)
			return err
		}
		_ = os.Remove(dockerConfigFile + ".e2e")
	}
	return nil
}
