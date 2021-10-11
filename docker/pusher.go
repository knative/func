package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	fn "knative.dev/kn-plugin-func"

	"github.com/containers/image/v5/pkg/docker/config"
	containersTypes "github.com/containers/image/v5/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
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
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

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

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("failed to create docker api client: %w", err)
	}

	n.progressListener.Stopping()
	credentials, err := n.credentialsProvider(ctx, registry)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}
	n.progressListener.Increment("Pushing function image to the registry")

	authConfig := types.AuthConfig{
		Username:      credentials.Username,
		Password:      credentials.Password,
		ServerAddress: registry,
	}

	b, err := json.Marshal(&authConfig)
	if err != nil {
		return "", err
	}

	opts := types.ImagePushOptions{RegistryAuth: base64.StdEncoding.EncodeToString(b)}

	r, err := cli.ImagePush(ctx, f.Image, opts)
	if err != nil {
		return "", fmt.Errorf("failed to push the image: %w", err)
	}
	defer r.Close()

	var output io.Writer
	var outBuff bytes.Buffer

	// If verbose logging is enabled, echo chatty stdout.
	if n.Verbose {
		output = io.MultiWriter(&outBuff, os.Stdout)
	} else {
		output = &outBuff
	}

	decoder := json.NewDecoder(r)
	li := logItem{}
	for {
		err = decoder.Decode(&li)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
		if li.Error != "" {
			return "", errors.New(li.ErrorDetail.Message)
		}
		if li.Id != "" {
			fmt.Fprintf(output, "%s: ", li.Id)
		}
		var percent int
		if li.ProgressDetail.Total == 0 {
			percent = 100
		} else {
			percent = (li.ProgressDetail.Current * 100) / li.ProgressDetail.Total
		}
		fmt.Fprintf(output, "%s (%d%%)\n", li.Status, percent)
	}

	digest = parseDigest(outBuff.String())

	return
}

var digestRE = regexp.MustCompile(`digest:\s+(sha256:\w{64})`)

// parseDigest tries to parse the last line from the output, which holds the pushed image digest
// The output should contain line like this:
// latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613
func parseDigest(output string) string {
	match := digestRE.FindStringSubmatch(output)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

type errorDetail struct {
	Message string `json:"message"`
}

type progressDetail struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

type logItem struct {
	Id             string         `json:"id"`
	Status         string         `json:"status"`
	Error          string         `json:"error"`
	ErrorDetail    errorDetail    `json:"errorDetail"`
	Progress       string         `json:"progress"`
	ProgressDetail progressDetail `json:"progressDetail"`
}
