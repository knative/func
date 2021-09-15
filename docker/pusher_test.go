package docker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
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

func Test_NewCredentialsProvider(t *testing.T) {
	// TODO add tests where also reading from config is utilized.
	defer withCleanHome(t)()

	ctx := context.Background()

	firstInvocation := true
	pwdCbk := func(registry string) (Credentials, error) {
		if registry != "docker.io" {
			return Credentials{}, fmt.Errorf("unexpected registry: %s", registry)
		}
		if firstInvocation {
			firstInvocation = false
			return Credentials{"testUser", "badPwd"}, nil
		}
		return Credentials{"testUser", "goodPwd"}, nil
	}

	verifyCbk := func(ctx context.Context, username, password, registry string) error {
		if username == "testUser" && password == "goodPwd" && registry == "docker.io" {
			return nil
		}
		return ErrUnauthorized
	}

	credentialProvider := NewCredentialsProvider(pwdCbk, verifyCbk)

	creds, err := credentialProvider(ctx, "docker.io")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	expectedCredentials := Credentials{Username: "testUser", Password: "goodPwd"}
	if creds != expectedCredentials {
		t.Errorf("credentialProvider() = %v, want %v", creds, expectedCredentials)
	}
}

func withCleanHome(t *testing.T) func() {
	t.Helper()
	homeName := "HOME"
	if runtime.GOOS == "windows" {
		homeName = "USERPROFILE"
	}
	tmpDir := t.TempDir()
	oldHome, hadHome := os.LookupEnv(homeName)
	os.Setenv(homeName, tmpDir)

	return func() {
		if hadHome {
			os.Setenv(homeName, oldHome)
		} else {
			os.Unsetenv(homeName)
		}
	}
}
