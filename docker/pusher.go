package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/docker/docker/client"

	fn "knative.dev/kn-plugin-func"

	"github.com/containers/image/v5/pkg/docker/config"
	containersTypes "github.com/containers/image/v5/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/errdefs"
)

type Opt func(*Pusher) error

type Credentials struct {
	Username string
	Password string
}

type CredentialsProvider func(ctx context.Context, registry string) (Credentials, error)

type CredentialsCallback func(registry string) (Credentials, error)

var ErrUnauthorized = errors.New("bad credentials")

// VerifyCredentialsCallback checks if credentials are accepted by the registry.
// If credentials are incorrect this callback shall return ErrUnauthorized.
type VerifyCredentialsCallback func(ctx context.Context, username, password, registry string) error

func CheckAuth(ctx context.Context, username, password, registry string) error {
	cli, _, err := NewClient(client.DefaultDockerHost)
	if err != nil {
		return err
	}
	defer cli.Close()

	_, err = cli.RegistryLogin(ctx, types.AuthConfig{Username: username, Password: password, ServerAddress: registry})
	if err != nil && strings.Contains(err.Error(), "401 Unauthorized") {
		return ErrUnauthorized
	}

	// podman hack until https://github.com/containers/podman/pull/11595 is merged
	// podman returns 400 (instead of 500) and body in unexpected shape
	if errdefs.IsInvalidParameter(err) {
		return ErrUnauthorized
	}

	return err
}

type ChooseCredentialHelperCallback func(available []string) (string, error)

// NewCredentialsProvider returns new CredentialsProvider that tires to get credentials from `docker` and `func` config files.
//
// In case getting credentials from the config files fails
// the caller provided callback will be invoked to obtain credentials.
// The callback may be called multiple times in case it returned credentials that are not correct (see verifyCredentials).
//
// When the callback succeeds the credentials will be saved by using helper defined in the `func` config.
// If the helper is not defined in the config the chooseCredentialHelper parameter will be used to pick one.
// The picked value will be saved in the func config.
//
// To verify that credentials are correct the verifyCredentials parameter is used.
// If verifyCredentials is not set then CheckAuth will be used as a fallback.
func NewCredentialsProvider(
	getCredentials CredentialsCallback,
	verifyCredentials VerifyCredentialsCallback,
	chooseCredentialHelper ChooseCredentialHelperCallback) CredentialsProvider {

	if verifyCredentials == nil {
		verifyCredentials = CheckAuth
	}

	if chooseCredentialHelper == nil {
		chooseCredentialHelper = func(available []string) (string, error) {
			return "", nil
		}
	}

	authFilePath := filepath.Join(fn.ConfigPath(), "auth.json")
	sys := &containersTypes.SystemContext{
		AuthFilePath: authFilePath,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
		//return result, fmt.Errorf("failed to determine home directory: %w", err)
	}
	dockerConfigPath := filepath.Join(home, ".docker", "config.json")

	return func(ctx context.Context, registry string) (Credentials, error) {
		result := Credentials{}

		for _, load := range []func() (containersTypes.DockerAuthConfig, error){
			func() (containersTypes.DockerAuthConfig, error) {
				return config.GetCredentials(sys, registry)
			},
			func() (containersTypes.DockerAuthConfig, error) {
				return getCredentialsByCredentialHelper(authFilePath, registry)
			},
			func() (containersTypes.DockerAuthConfig, error) {
				return getCredentialsByCredentialHelper(dockerConfigPath, registry)
			},
		} {
			var credentials containersTypes.DockerAuthConfig
			credentials, err = load()

			if err != nil && !errors.Is(err, errCredentialsNotFound) {
				return Credentials{}, err
			}

			if credentials != (containersTypes.DockerAuthConfig{}) {
				result.Username, result.Password = credentials.Username, credentials.Password

				err = verifyCredentials(ctx, result.Username, result.Password, registry)
				if err == nil {
					return result, nil
				} else {
					if !errors.Is(err, ErrUnauthorized) {
						return Credentials{}, err
					}
				}
			}
		}

		for {
			result, err = getCredentials(registry)
			if err != nil {
				return Credentials{}, err
			}

			err = verifyCredentials(ctx, result.Username, result.Password, registry)
			if err == nil {
				err = setCredentialsByCredentialHelper(authFilePath, registry, result.Username, result.Password)
				if err != nil {
					if !errors.Is(err, errNoCredentialHelperConfigured) {
						return Credentials{}, err
					}
					helpers := listCredentialHelpers()
					helper, err := chooseCredentialHelper(helpers)
					if err != nil {
						return Credentials{}, err
					}
					helper = strings.TrimPrefix(helper, "docker-credential-")
					err = setCredentialHelperToConfig(authFilePath, helper)
					if err != nil {
						return Credentials{}, fmt.Errorf("faild to set the helper to the config: %w", err)
					}
					err = setCredentialsByCredentialHelper(authFilePath, registry, result.Username, result.Password)
					if err != nil && !errors.Is(err, errNoCredentialHelperConfigured) {
						return Credentials{}, err
					}
				}
				return result, nil
			} else {
				if errors.Is(err, ErrUnauthorized) {
					continue
				}
				return Credentials{}, err
			}
		}
	}
}

// Pusher of images from local to remote registry.
type Pusher struct {
	// Verbose logging.
	Verbose             bool
	credentialsProvider CredentialsProvider
	progressListener    fn.ProgressListener
}

func WithCredentialsProvider(cp CredentialsProvider) Opt {
	return func(p *Pusher) error {
		p.credentialsProvider = cp
		return nil
	}
}

func WithProgressListener(pl fn.ProgressListener) Opt {
	return func(p *Pusher) error {
		p.progressListener = pl
		return nil
	}
}

func EmptyCredentialsProvider(ctx context.Context, registry string) (Credentials, error) {
	return Credentials{}, nil
}

// NewPusher creates an instance of a docker-based image pusher.
func NewPusher(opts ...Opt) (*Pusher, error) {
	result := &Pusher{
		Verbose:             false,
		credentialsProvider: EmptyCredentialsProvider,
		progressListener:    &fn.NoopProgressListener{},
	}
	for _, opt := range opts {
		err := opt(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func getRegistry(image_url string) (string, error) {
	var registry string
	parts := strings.Split(image_url, "/")
	switch {
	case len(parts) == 2:
		registry = fn.DefaultRegistry
	case len(parts) >= 3:
		registry = parts[0]
	default:
		return "", fmt.Errorf("failed to parse image name: %q", image_url)
	}

	return registry, nil
}

// Push the image of the Function.
func (n *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {

	if f.Image == "" {
		return "", errors.New("Function has no associated image.  Has it been built?")
	}

	registry, err := getRegistry(f.Image)
	if err != nil {
		return "", err
	}

	cli, _, err := NewClient(client.DefaultDockerHost)
	if err != nil {
		return "", fmt.Errorf("failed to create docker api client: %w", err)
	}
	defer cli.Close()

	n.progressListener.Stopping()
	credentials, err := n.credentialsProvider(ctx, registry)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}
	n.progressListener.Increment("Pushing function image to the registry")

	auth := &authn.Basic{
		Username: credentials.Username,
		Password: credentials.Password,
	}

	progressChannel := make(chan v1.Update)
	opts := []remote.Option{
		remote.WithAuth(auth),
		remote.WithProgress(progressChannel),
	}

	var output io.Writer
	var outBuff bytes.Buffer

	// If verbose logging is enabled, echo chatty stdout.
	if n.Verbose {
		output = io.MultiWriter(&outBuff, os.Stdout)
	} else {
		output = &outBuff
	}

	ref, err := name.ParseReference(f.Image)
	if err != nil {
		return "", err
	}

	img, err := daemon.Image(ref)
	if err != nil {
		return "", err
	}

	go func() {
		for progress := range progressChannel {
			fmt.Fprintf(output, "progress: %+v\n", progress)
		}
	}()

	err = remote.Write(ref, img, opts...)
	if err != nil {
		return "", err
	}

	hash, err := img.Digest()
	if err != nil {
		return "", err
	}

	return hash.String(), nil
}
