//go:build e2e && !windows

package e2e

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/test/common"
	"knative.dev/func/test/testhttp"
)

// setupConfigEnvsTest add to cluster config maps and secrets used by the test
func setupConfigEnvsTest(t *testing.T) {

	config, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	namespace, _, _ := k8s.GetClientConfig().Namespace()

	// Add Config Map
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
		Data: map[string]string{
			"TEST_CM_MSG1": "Hi",
			"TEST_CM_MSG2": "Hello",
		},
	}
	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(context.Background(), &configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Add Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret"},
		Data: map[string][]byte{
			"TEST_SECRET_PW1": []byte("pw1"),
			"TEST_SECRET_PW2": []byte("pw2"),
		},
	}
	_, err = clientset.CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

}

// tearDownConfigEnvsTest removes cluster config maps and secrets used by the test
func tearDownConfigEnvsTest() {

	config, _ := k8s.GetClientConfig().ClientConfig()
	clientset, _ := kubernetes.NewForConfig(config)
	namespace, _, _ := k8s.GetClientConfig().Namespace()

	_ = clientset.CoreV1().ConfigMaps(namespace).Delete(context.Background(), "test-cm", metav1.DeleteOptions{})
	_ = clientset.CoreV1().Secrets(namespace).Delete(context.Background(), "test-secret", metav1.DeleteOptions{})

}

// ConfigEnvsAdd generate sa go function to test `func config labels add` with user input
func ConfigEnvsAdd(knFunc *common.TestInteractiveCmd, functionPath string) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "envs", "add", "--path", functionPath)
}

// ConfigEnvsRemove generates a go function to test `func config labels remove` with user input
func ConfigEnvsRemove(knFunc *common.TestInteractiveCmd, functionPath string) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "envs", "remove", "--path", functionPath)
}

// TestConfigEnvs verifies function environment variables are properly set on the deployed functions.
// Test consist in explore all available options to add environment variables and ensure they get deployed
// It setup "configMaps" and "secrets" on the cluster. A custom kn function template (from a remote repository)
// is used to validate the environment variables are properly resolved.
func TestConfigEnvs(t *testing.T) {

	setupConfigEnvsTest(t)
	defer tearDownConfigEnvsTest()

	testEnvName := "TEST_ENV"
	testEnvValue := "TEST_VALUE"

	knFunc := common.NewTestShellInteractiveCmd(t)
	knFunc.TestCmd.ShouldDumpOnSuccess = false
	knFunc.CommandSleepInterval = time.Millisecond * 1500

	// On When...
	funcName := "test-config-envs"
	funcPath := filepath.Join(t.TempDir(), funcName)

	_, thisfile, _, _ := runtime.Caller(0)
	testTemplateFolder := path.Join(path.Dir(thisfile), "..", "templates")

	knFunc.TestCmd.Exec("create",
		"--language", "go",
		"--template", "testenvs",
		"--repository", "file://"+testTemplateFolder,
		funcPath)
	knFunc.TestCmd.SourceDir = funcPath

	/*
		Config Envs Add command prompts user to add envs with below options:
			? What type of Environment variable do you want to add?  [Use arrows to move, type to filter]
			> Environment variable with a specified value
			  Value from a local environment variable
			  ConfigMap: all key=value pairs as environment variables
			  ConfigMap: value from a key
			  Secret: all key=value pairs as environment variables
			  Secret: value from a key
	*/
	configEnvsAdd := ConfigEnvsAdd(knFunc, funcPath)

	configEnvsAdd(
		enter,                // Environment variable with a specified value
		"TEST_ENV_SV", enter, // env var name
		"V1", enter) // env var value

	configEnvsAdd(
		enter,
		arrowDown, enter, // Value from a local environment variable
		"TEST_ENV_LEV", enter, // env var name
		testEnvName, enter) // local env var name

	configEnvsAdd(
		enter,
		"ConfigMap: all", enter, // ConfigMap: all key=value pairs as environment variables
		"test-cm", enter) // config map name

	configEnvsAdd(
		enter,
		"ConfigMap: value", enter, // ConfigMap: value from a key
		"test-cm", enter, // config map name
		"TEST_ENV_CMK", enter, // env var name
		"TEST_CM_MSG1", enter) // key from config map

	configEnvsAdd(
		enter,
		"Secret: all", enter, // Secret: all key=value pairs as environment variables
		"test-secret", enter) // secret name

	configEnvsAdd(
		enter,
		"Secret: value", enter, // Secret: value from a key
		"test-secret", enter, // secret name
		"TEST_ENV_SK", enter, // env var name
		"TEST_SECRET_PW1", enter) // key from secret

	// Another "value from a local environment variable" in order to be deleted
	configEnvsAdd(enter, arrowDown, enter, "TEST_WRONG_ENV", enter, "TEST_ENV", enter)

	// Delete last Env var entered
	configEnvsRemove := ConfigEnvsRemove(knFunc, funcPath)
	configEnvsRemove("TEST_WRONG_ENV", enter)

	// Deploy
	knFunc.TestCmd.WithEnv(testEnvName, testEnvValue)
	knFunc.TestCmd.Exec("deploy", "--registry", common.GetRegistry())
	defer knFunc.TestCmd.Exec("delete")
	_, functionUrl := common.WaitForFunctionReady(t, funcName)

	// Validate
	// The function template used by this test will return all
	// environment variable started with TEST_ on default endpoint
	envValidator := func(statusCode int, responseBody string) error {
		if responseBody == "" {
			return fmt.Errorf("expected response body on deployed function")
		}
		envs := map[string]string{}
		for _, kv := range strings.Split(responseBody, "\n") {
			s := strings.Split(kv, "=")
			if len(s) == 2 {
				envs[s[0]] = s[1]
			}
		}
		expectedEnvs := map[string]string{
			"TEST_ENV_SV":     "V1",
			"TEST_ENV_LEV":    testEnvValue,
			"TEST_CM_MSG1":    "Hi",
			"TEST_CM_MSG2":    "Hello",
			"TEST_ENV_CMK":    "Hi",
			"TEST_SECRET_PW1": "pw1",
			"TEST_SECRET_PW2": "pw2",
			"TEST_ENV_SK":     "pw1",
		}

		var result = ""
		for expectedEnv, expectedValue := range expectedEnvs {
			if envs[expectedEnv] != expectedValue {
				result = fmt.Sprintf("%vexpected env [%v] with value [%v], but got [%v]\n", result, expectedEnv, expectedValue, envs[expectedEnv])
			}
		}
		if envs["TEST_WRONG_ENV"] != "" {
			result = fmt.Sprintf("%vunexpected env [%v] was found", result, "TEST_WRONG_ENV")
		}
		if result != "" {
			t.Logf("Response received:\n%v", responseBody)
			return fmt.Errorf(result)
		}
		return nil
	}
	statusCode, funcResponse := testhttp.TestGet(t, functionUrl)
	assert.NilError(t, envValidator(statusCode, funcResponse))
}
