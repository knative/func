package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker-credential-helpers/credentials"
	fn "knative.dev/kn-plugin-func"
)

func Test_parseDigest(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "basic test",
			arg:  "latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613",
			want: "sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDigest(tt.arg); got != tt.want {
				t.Errorf("parseDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getRegistry(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "default registry",
			arg:  "docker.io/mysamplefunc:latest",
			want: "docker.io",
		},
		{
			name: "long-form nested url",
			arg:  "myregistry.io/myorg/myuser/myfunctions/mysamplefunc:latest",
			want: "myregistry.io",
		},
		{
			name: "invalid url",
			arg:  "myregistry.io-mysamplefunc:latest",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := getRegistry(tt.arg); got != tt.want {
				t.Errorf("getRegistry() = %v, want %v", got, tt.want)
			}
		})
	}
}

const (
	dockerIoUser    = "testUser1"
	dockerIoUserPwd = "goodPwd1"
	quayIoUser      = "testUser2"
	quayIoUserPwd   = "goodPwd2"
)

func TestNewCredentialsProvider(t *testing.T) {
	defer withCleanHome(t)()

	helperWithQuayIO := newInMemoryHelper()

	err := helperWithQuayIO.Add(&credentials.Credentials{
		ServerURL: "quay.io",
		Username:  quayIoUser,
		Secret:    quayIoUserPwd,
	})
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		credentialsCallback CredentialsCallback
		verifyCredentials   VerifyCredentialsCallback
		registry            string
		setUpEnv            setUpEnv
	}
	tests := []struct {
		name string
		args args
		want Credentials
	}{
		{
			name: "test user callback correct password on first try",
			args: args{
				credentialsCallback: correctPwdCallback,
				verifyCredentials:   correctVerifyCbk,
				registry:            "docker.io",
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "test user callback correct password on second try",
			args: args{
				credentialsCallback: pwdCbkFirstWrongThenCorrect(t),
				verifyCredentials:   correctVerifyCbk,
				registry:            "docker.io",
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "get quay-io credentials with func config populated",
			args: args{
				credentialsCallback: pwdCbkThatShallNotBeCalled(t),
				verifyCredentials:   correctVerifyCbk,
				registry:            "quay.io",
				setUpEnv:            withPopulatedFuncAuthConfig,
			},
			want: Credentials{Username: quayIoUser, Password: quayIoUserPwd},
		},
		{
			name: "get docker-io credentials with func config populated",
			args: args{
				credentialsCallback: pwdCbkThatShallNotBeCalled(t),
				verifyCredentials:   correctVerifyCbk,
				registry:            "docker.io",
				setUpEnv:            withPopulatedFuncAuthConfig,
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
		{
			name: "get quay-io credentials with docker config populated",
			args: args{
				credentialsCallback: pwdCbkThatShallNotBeCalled(t),
				verifyCredentials:   correctVerifyCbk,
				registry:            "quay.io",
				setUpEnv: all(
					withPopulatedDockerAuthConfig,
					setUpMockHelper("docker-credential-mock", helperWithQuayIO)),
			},
			want: Credentials{Username: quayIoUser, Password: quayIoUserPwd},
		},
		{
			name: "get docker-io credentials with docker config populated",
			args: args{
				credentialsCallback: pwdCbkThatShallNotBeCalled(t),
				verifyCredentials:   correctVerifyCbk,
				registry:            "docker.io",
				setUpEnv:            withPopulatedDockerAuthConfig,
			},
			want: Credentials{Username: dockerIoUser, Password: dockerIoUserPwd},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanUpConfigs(t)

			if tt.args.setUpEnv != nil {
				defer tt.args.setUpEnv(t)()
			}

			credentialsProvider := NewCredentialsProvider(tt.args.credentialsCallback, tt.args.verifyCredentials, nil)
			got, err := credentialsProvider(context.Background(), tt.args.registry)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("credentialsProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialsProviderSavingFromUserInput(t *testing.T) {
	defer withCleanHome(t)()

	helper := newInMemoryHelper()
	defer setUpMockHelper("docker-credential-mock", helper)(t)()

	var pwdCbkInvocations int
	pwdCbk := func(r string) (Credentials, error) {
		pwdCbkInvocations++
		return correctPwdCallback(r)
	}

	chooseNoStore := func(available []string) (string, error) {
		if len(available) < 1 {
			t.Errorf("this should have been invoked with non empty list")
		}
		return "", nil
	}
	chooseMockStore := func(available []string) (string, error) {
		if len(available) < 1 {
			t.Errorf("this should have been invoked with non empty list")
		}
		return "docker-credential-mock", nil
	}
	shallNotBeInvoked := func(available []string) (string, error) {
		t.Fatal("this choose helper callback shall not be invoked")
		return "", errors.New("this callback shall not be invoked")
	}

	credentialsProvider := NewCredentialsProvider(pwdCbk, correctVerifyCbk, chooseNoStore)
	_, err := credentialsProvider(context.Background(), "docker.io")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// now credentials should not be saved because no helper was provided
	l, err := helper.List()
	if err != nil {
		t.Fatal(err)
	}
	credsInStore := len(l)
	if credsInStore != 0 {
		t.Errorf("expected to have zero credentials in store, but has: %d", credsInStore)
	}
	credentialsProvider = NewCredentialsProvider(pwdCbk, correctVerifyCbk, chooseMockStore)
	_, err = credentialsProvider(context.Background(), "docker.io")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	if pwdCbkInvocations != 2 {
		t.Errorf("the pwd callback should have been invoked exactly twice but was invoked %d time", pwdCbkInvocations)
	}

	// now credentials should be saved in the mock secure store
	l, err = helper.List()
	if err != nil {
		t.Fatal(err)
	}
	credsInStore = len(l)
	if len(l) != 1 {
		t.Errorf("expected to have exactly one credentials in store, but has: %d", credsInStore)
	}
	credentialsProvider = NewCredentialsProvider(pwdCbkThatShallNotBeCalled(t),
		correctVerifyCbk,
		shallNotBeInvoked)
	_, err = credentialsProvider(context.Background(), "docker.io")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
}

func cleanUpConfigs(t *testing.T) {
	home, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}

	os.RemoveAll(fn.ConfigPath())

	os.RemoveAll(filepath.Join(home, ".docker"))
}

type setUpEnv = func(t *testing.T) func()

func withPopulatedDockerAuthConfig(t *testing.T) func() {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dockerConfigDir := filepath.Join(home, ".docker")
	dockerConfigPath := filepath.Join(dockerConfigDir, "config.json")
	err = os.MkdirAll(filepath.Dir(dockerConfigPath), 0700)
	if err != nil {
		t.Fatal(err)
	}

	configJSON := `{
	"auths": {
		"docker.io": { "auth": "%s" },
		"quay.io": {}
	},
	"credsStore": "mock"
}`
	configJSON = fmt.Sprintf(configJSON, base64.StdEncoding.EncodeToString([]byte(dockerIoUser+":"+dockerIoUserPwd)))

	err = ioutil.WriteFile(dockerConfigPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatal(err)
	}

	return func() {

		os.RemoveAll(dockerConfigDir)
	}
}

func withPopulatedFuncAuthConfig(t *testing.T) func() {
	t.Helper()

	var err error

	authConfig := filepath.Join(fn.ConfigPath(), "auth.json")
	err = os.MkdirAll(filepath.Dir(authConfig), 0700)
	if err != nil {
		t.Fatal(err)
	}

	authJSON := `{
	"auths": {
		"docker.io": { "auth": "%s" },
		"quay.io":   { "auth": "%s" }
	}
}`
	authJSON = fmt.Sprintf(authJSON,
		base64.StdEncoding.EncodeToString([]byte(dockerIoUser+":"+dockerIoUserPwd)),
		base64.StdEncoding.EncodeToString([]byte(quayIoUser+":"+quayIoUserPwd)))

	err = ioutil.WriteFile(authConfig, []byte(authJSON), 0600)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		os.RemoveAll(fn.ConfigPath())
	}
}

func pwdCbkThatShallNotBeCalled(t *testing.T) CredentialsCallback {
	t.Helper()
	return func(registry string) (Credentials, error) {
		return Credentials{}, errors.New("this pwd cbk code shall not be called")
	}
}

func pwdCbkFirstWrongThenCorrect(t *testing.T) func(registry string) (Credentials, error) {
	t.Helper()
	var firstInvocation bool
	return func(registry string) (Credentials, error) {
		if registry != "docker.io" && registry != "quay.io" {
			return Credentials{}, fmt.Errorf("unexpected registry: %s", registry)
		}
		if firstInvocation {
			firstInvocation = false
			return Credentials{dockerIoUser, "badPwd"}, nil
		}
		return correctPwdCallback(registry)
	}
}

func correctPwdCallback(registry string) (Credentials, error) {
	if registry == "docker.io" {
		return Credentials{Username: dockerIoUser, Password: dockerIoUserPwd}, nil
	}
	if registry == "quay.io" {
		return Credentials{Username: quayIoUser, Password: quayIoUserPwd}, nil
	}
	return Credentials{}, errors.New("this cbk don't know the pwd")
}

func correctVerifyCbk(ctx context.Context, username, password, registry string) error {
	if username == dockerIoUser && password == dockerIoUserPwd && registry == "docker.io" {
		return nil
	}
	if username == quayIoUser && password == quayIoUserPwd && registry == "quay.io" {
		return nil
	}
	return ErrUnauthorized
}

func withCleanHome(t *testing.T) func() {
	t.Helper()
	homeName := "HOME"
	if runtime.GOOS == "windows" {
		homeName = "USERPROFILE"
	}
	tmpHome := t.TempDir()
	oldHome, hadHome := os.LookupEnv(homeName)
	os.Setenv(homeName, tmpHome)

	oldXDGConfigHome, hadXDGConfigHome := os.LookupEnv("XDG_CONFIG_HOME")

	if runtime.GOOS == "linux" {
		os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))
	}

	return func() {
		if hadHome {
			os.Setenv(homeName, oldHome)
		} else {
			os.Unsetenv(homeName)
		}
		if hadXDGConfigHome {
			os.Setenv("XDG_CONFIG_HOME", oldXDGConfigHome)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}
}

func handlerForCredHelper(t *testing.T, credHelper credentials.Helper) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		var err error
		var outBody interface{}

		uri := strings.Trim(request.RequestURI, "/")

		var serverURL string
		if uri == "get" || uri == "erase" {
			data, err := ioutil.ReadAll(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			serverURL = string(data)
			serverURL = strings.Trim(serverURL, "\n\r\t ")
		}

		switch uri {
		case "list":
			var list map[string]string
			list, err = credHelper.List()
			if err == nil {
				outBody = &list
			}
		case "store":
			creds := credentials.Credentials{}
			dec := json.NewDecoder(request.Body)
			err = dec.Decode(&creds)
			if err != nil {
				break
			}
			err = credHelper.Add(&creds)
		case "get":
			var user, secret string
			user, secret, err = credHelper.Get(serverURL)
			if err == nil {
				outBody = &credentials.Credentials{ServerURL: serverURL, Username: user, Secret: secret}
			}
		case "erase":
			err = credHelper.Delete(serverURL)
		default:
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		if err != nil {
			if credentials.IsErrCredentialsNotFound(err) {
				writer.WriteHeader(http.StatusNotFound)
			} else {
				writer.WriteHeader(http.StatusInternalServerError)
				writer.Header().Add("Content-Type", "text/plain")
				fmt.Fprintf(writer, "error: %+v\n", err)
			}
			return
		}

		if outBody != nil {
			var data []byte
			data, err = json.Marshal(outBody)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			writer.Header().Add("Content-Type", "application/json")
			_, err = writer.Write(data)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

}

const helperGoScriptContent = `package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
)

var baseURL = "http://HOST_PORT"

func main() {
	var resp *http.Response
	var err error
	cmd := os.Args[1]
	switch cmd {
	case "list":
		resp, err = http.Get(baseURL + "/" + cmd)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(os.Stdout, resp.Body)
	case "get", "erase":
		resp, err = http.Post(baseURL+ "/" + cmd, "text/plain", os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(os.Stdout, resp.Body)
	case "store":
		resp, err = http.Post(baseURL+ "/" + cmd, "application/json", os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal(errors.New("unknown cmd: " + cmd))
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatal(errors.New(resp.Status))
	}
	return
}
`

// Creates executable with name determined by the helperName parameter and puts it on $PATH.
//
// The executable behaves like docker credential helper (https://github.com/docker/docker-credential-helpers).
//
// The content of the store presented by the executable is backed by the helper parameter.
func setUpMockHelper(helperName string, helper credentials.Helper) func(t *testing.T) func() {
	return func(t *testing.T) func() {

		listener, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			t.Fatal(err)
		}

		hostPort := listener.Addr().String()

		server := http.Server{Handler: handlerForCredHelper(t, helper)}
		servErrChan := make(chan error)
		go func() {
			servErrChan <- server.Serve(listener)
		}()

		binDir, err := ioutil.TempDir("", "binDirForCredHelper")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "cd %s\n", binDir)

		helperGoScriptPath := filepath.Join(binDir, "main.go")

		err = ioutil.WriteFile(helperGoScriptPath,
			[]byte(strings.ReplaceAll(helperGoScriptContent, "HOST_PORT", hostPort)),
			0400)
		if err != nil {
			t.Fatal(err)
		}

		runnerScriptName := helperName
		if runtime.GOOS == "windows" {
			runnerScriptName = runnerScriptName + ".bat"
		}

		runnerScriptPath := filepath.Join(binDir, runnerScriptName)

		runnerScriptContent := `#!/bin/sh
exec go run GO_SCRIPT_PATH $@;
`
		if runtime.GOOS == "windows" {
			runnerScriptContent = `@echo off
go.exe run GO_SCRIPT_PATH %*
`
		}

		runnerScriptContent = strings.ReplaceAll(runnerScriptContent, "GO_SCRIPT_PATH", helperGoScriptPath)
		err = ioutil.WriteFile(runnerScriptPath, []byte(runnerScriptContent), 0700)
		if err != nil {
			t.Fatal(err)
		}

		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

		return func() {
			os.Setenv("PATH", oldPath)
			os.RemoveAll(binDir)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			_ = server.Shutdown(ctx)
			err := <-servErrChan
			if !errors.Is(err, http.ErrServerClosed) {
				t.Fatal(err)
			}
		}
	}
}

// combines multiple setUp routines into one setUp routine
func all(fns ...setUpEnv) setUpEnv {
	return func(t *testing.T) func() {
		t.Helper()
		var cleanUps []func()
		for _, fn := range fns {
			cleanUps = append(cleanUps, fn(t))
		}

		return func() {
			for i := len(cleanUps) - 1; i >= 0; i-- {
				cleanUps[i]()
			}
		}
	}
}

func newInMemoryHelper() *inMemoryHelper {
	return &inMemoryHelper{lock: &sync.Mutex{}, credentials: make(map[string]credentials.Credentials)}
}

type inMemoryHelper struct {
	credentials map[string]credentials.Credentials
	lock        sync.Locker
}

func (i *inMemoryHelper) Add(credentials *credentials.Credentials) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.credentials[credentials.ServerURL] = *credentials

	return nil
}

func (i *inMemoryHelper) Get(serverURL string) (string, string, error) {
	i.lock.Lock()
	defer i.lock.Unlock()

	if result, ok := i.credentials[serverURL]; ok {
		return result.Username, result.Secret, nil
	}

	return "", "", credentials.NewErrCredentialsNotFound()
}

func (i *inMemoryHelper) List() (map[string]string, error) {
	i.lock.Lock()
	defer i.lock.Unlock()

	result := make(map[string]string, len(i.credentials))

	for k, v := range i.credentials {
		result[k] = v.Username
	}

	return result, nil
}

func (i *inMemoryHelper) Delete(serverURL string) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if _, ok := i.credentials[serverURL]; ok {
		delete(i.credentials, serverURL)
		return nil
	}

	return credentials.NewErrCredentialsNotFound()
}
