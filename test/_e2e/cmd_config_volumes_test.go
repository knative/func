//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/kn-plugin-func/builders"
	"knative.dev/kn-plugin-func/k8s"
)

// setupConfigVolumesTest add to cluster config maps and secrets that will be used as volumes
// during tests
func setupConfigVolumesTest(t *testing.T) {

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
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm-volume"},
		Data: map[string]string{
			"config-key1": "Hi",
			"config-key2": "Hello",
		},
	}
	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(context.Background(), &configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Add Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret-volume"},
		Data: map[string][]byte{
			"secret-key1": []byte("pw1"),
			"secret-key2": []byte("pw2"),
		},
	}
	_, err = clientset.CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

}

// tearDownConfigVolumesTest removes cluster config maps and secrets used by the test
func tearDownConfigVolumesTest() {

	config, _ := k8s.GetClientConfig().ClientConfig()
	clientset, _ := kubernetes.NewForConfig(config)
	namespace, _, _ := k8s.GetClientConfig().Namespace()

	_ = clientset.CoreV1().ConfigMaps(namespace).Delete(context.Background(), "test-cm-volume", metav1.DeleteOptions{})
	_ = clientset.CoreV1().Secrets(namespace).Delete(context.Background(), "test-secret-volume", metav1.DeleteOptions{})

}

// ConfigVolumesAdd generates a go function to test `func config volumes add` with user input
func ConfigVolumesAdd(knFunc *TestShellInteractiveCmdRunner, project *FunctionTestProject) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "volumes", "add", "--path", project.ProjectPath)
}

// ConfigVolumesRemove generates a go function to test `func config volumes remove` with user input
func ConfigVolumesRemove(knFunc *TestShellInteractiveCmdRunner, project *FunctionTestProject) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "volumes", "remove", "--path", project.ProjectPath)
}

// TestConfigVolumes verifies configMaps and secrets were properly mounted as volumes and accessible to the function
// Test consist reproduce the user experience to add volumes (both config and secrets) and deploy a function that
// makes use of the data/
// It setup "configMaps" and "secrets" on the cluster. A custom kn Function template (from a remote repository)
// is used to validate the data can be accessed from the deployed function perspective.
func TestConfigVolumes(t *testing.T) {

	setupConfigVolumesTest(t)
	defer tearDownConfigVolumesTest()

	knFunc := NewTestShellInteractiveCmdRunner(t)
	knFunc.TestShell.ShouldDumpOnSuccess = false
	knFunc.commandSleepInterval = time.Millisecond * 1500

	// On When...
	project := FunctionTestProject{}
	project.Runtime = "go"
	project.Template = "volumes"
	project.FunctionName = "test-config-volumes"
	project.ProjectPath = filepath.Join(os.TempDir(), project.FunctionName)
	project.RemoteRepository = "http://github.com/boson-project/test-templates.git"
	project.Builder = builders.Pack

	Create(t, knFunc.TestShell, project)
	defer project.RemoveProjectFolder()

	/*
		? What do you want to mount as a Volume?  [Use arrows to move, type to filter]
		> ConfigMap
		  Secret
	*/
	configVolumesAdd := ConfigVolumesAdd(knFunc, &project)

	configVolumesAdd(
		enter,                   // > ConfigMap
		"test-cm-volume", enter, // Which "ConfigMap" do you want to mount?
		"/test/cm-volume", enter) // Please specify the path where the ConfigMap should be mounted:

	configVolumesAdd(
		arrowDown, enter, // > Secret
		"test-secret-volume", enter, // Which "Secret" do you want to mount?
		"/test/secret-volume", enter) // Please specify the path where the Secret should be mounted:

	// Adding unwanted volume entries (to simulate user mistakes)
	configVolumesAdd(
		enter,
		"test-cm-volume", enter,
		"/test/bad-cm", enter)

	configVolumesAdd(
		arrowDown, enter,
		"test-secret-volume", enter,
		"/test/bad-secret", enter)

	// Delete unwanted entries
	configVolumesRemove := ConfigVolumesRemove(knFunc, &project)
	configVolumesRemove("/bad-secret", enter)
	configVolumesRemove("/bad-cm", enter)

	// Deploy
	Build(t, knFunc.TestShell, &project)
	Deploy(t, knFunc.TestShell, &project)
	defer Delete(t, knFunc.TestShell, &project)
	ReadyCheck(t, knFunc.TestShell, project)

	// Validate
	// The function template used by this test will return
	// file content for the file specified as a query parameter named 'v'
	expectedMap := map[string]string{
		"/test/cm-volume/config-key1":     "Hi",
		"/test/cm-volume/config-key2":     "Hello",
		"/test/secret-volume/secret-key1": "pw1",
		"/test/secret-volume/secret-key2": "pw2",
		"/test/bad-cm/config-key1":        "",
		"/test/bad-secret/secret-key1":    "",
	}
	functionRespValidator := FunctionHttpResponsivenessValidator{runtime: "go"}
	for expectedVolumeEntry, expectedFileContent := range expectedMap {
		functionRespValidator.targetUrl = "%v?v=" + expectedVolumeEntry
		functionRespValidator.expects = expectedFileContent
		functionRespValidator.Validate(t, project)
	}

}
