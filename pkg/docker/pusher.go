package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"

	fn "knative.dev/func/pkg/functions"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/term"
)

type Opt func(*Pusher)

type Credentials struct {
	Username string
	Password string
}

type CredentialsProvider func(ctx context.Context, image string) (Credentials, error)

// PusherDockerClient is sub-interface of client.CommonAPIClient required by pusher.
type PusherDockerClient interface {
	daemon.Client
	ImagePush(ctx context.Context, ref string, options types.ImagePushOptions) (io.ReadCloser, error)
	Close() error
}

type PusherDockerClientFactory func() (PusherDockerClient, error)

// Pusher of images from local to remote registry.
type Pusher struct {
	verbose             bool // verbose logging.
	credentialsProvider CredentialsProvider
	transport           http.RoundTripper
	dockerClientFactory PusherDockerClientFactory
}

func WithCredentialsProvider(cp CredentialsProvider) Opt {
	return func(p *Pusher) {
		p.credentialsProvider = cp
	}
}

func WithTransport(transport http.RoundTripper) Opt {
	return func(pusher *Pusher) {
		pusher.transport = transport
	}
}

func WithPusherDockerClientFactory(dockerClientFactory PusherDockerClientFactory) Opt {
	return func(pusher *Pusher) {
		pusher.dockerClientFactory = dockerClientFactory
	}
}

func WithVerbose(verbose bool) Opt {
	return func(pusher *Pusher) {
		pusher.verbose = verbose
	}
}

func EmptyCredentialsProvider(ctx context.Context, registry string) (Credentials, error) {
	return Credentials{}, nil
}

// NewPusher creates an instance of a docker-based image pusher.
func NewPusher(opts ...Opt) *Pusher {
	result := &Pusher{
		credentialsProvider: EmptyCredentialsProvider,
		transport:           http.DefaultTransport,
		dockerClientFactory: func() (PusherDockerClient, error) {
			c, _, err := NewClient(client.DefaultDockerHost)
			return c, err
		},
	}
	for _, opt := range opts {
		opt(result)
	}

	return result
}

func GetRegistry(img string) (string, error) {
	ref, err := name.ParseReference(img, name.WeakValidation)
	if err != nil {
		return "", err
	}
	registry := ref.Context().RegistryStr()
	return registry, nil
}

// Push the image of the function.
func (n *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {

	var output io.Writer

	if n.verbose {
		output = os.Stderr
	} else {
		output = io.Discard
	}

	if f.Build.Image == "" {
		return "", errors.New("Function has no associated image.  Has it been built?")
	}

	registry, err := GetRegistry(f.Build.Image)
	if err != nil {
		return "", err
	}

	credentials, err := n.credentialsProvider(ctx, f.Build.Image)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Pushing function image to the registry %q using the %q user credentials\n", registry, credentials.Username)

	// if the registry is not cluster private do push directly from daemon
	if _, err = net.DefaultResolver.LookupHost(ctx, registry); err == nil {
		return n.daemonPush(ctx, f, credentials, output)
	}

	// push with custom transport to be able to push into cluster private registries
	return n.push(ctx, f, credentials, output)
}

func (n *Pusher) daemonPush(ctx context.Context, f fn.Function, credentials Credentials, output io.Writer) (digest string, err error) {
	cli, err := n.dockerClientFactory()
	if err != nil {
		return "", fmt.Errorf("failed to create docker api client: %w", err)
	}
	defer cli.Close()

	authConfig := registry.AuthConfig{
		Username: credentials.Username,
		Password: credentials.Password,
	}

	b, err := json.Marshal(&authConfig)
	if err != nil {
		return "", err
	}

	opts := types.ImagePushOptions{RegistryAuth: base64.StdEncoding.EncodeToString(b)}

	r, err := cli.ImagePush(ctx, f.Build.Image, opts)
	if err != nil {
		return "", fmt.Errorf("failed to push the image: %w", err)
	}
	defer r.Close()

	var outBuff bytes.Buffer
	output = io.MultiWriter(&outBuff, output)

	var isTerminal bool
	var fd uintptr
	if outF, ok := output.(*os.File); ok {
		fd = outF.Fd()
		isTerminal = term.IsTerminal(int(outF.Fd()))
	}

	err = jsonmessage.DisplayJSONMessagesStream(r, output, fd, isTerminal, nil)
	if err != nil {
		return "", err
	}

	return ParseDigest(outBuff.String()), nil
}

var digestRE = regexp.MustCompile(`digest:\s+(sha256:\w{64})`)

// ParseDigest tries to parse the last line from the output, which holds the pushed image digest
// The output should contain line like this:
// latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613
func ParseDigest(output string) string {
	match := digestRE.FindStringSubmatch(output)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

func (n *Pusher) push(ctx context.Context, f fn.Function, credentials Credentials, output io.Writer) (digest string, err error) {
	auth := &authn.Basic{
		Username: credentials.Username,
		Password: credentials.Password,
	}

	ref, err := name.ParseReference(f.Build.Image)
	if err != nil {
		return "", err
	}

	dockerClient, err := n.dockerClientFactory()
	if err != nil {
		return "", fmt.Errorf("failed to create docker api client: %w", err)
	}
	defer dockerClient.Close()

	img, err := daemon.Image(ref,
		daemon.WithContext(ctx),
		daemon.WithClient(dockerClient))
	if err != nil {
		return "", err
	}

	progressChannel := make(chan v1.Update, 1024)
	errChan := make(chan error)
	go func() {
		defer fmt.Fprint(output, "\n")

		for progress := range progressChannel {
			if progress.Error != nil {
				errChan <- progress.Error
				return
			}
			fmt.Fprintf(output, "\rprogress: %d%%", progress.Complete*100/progress.Total)
		}

		errChan <- nil
	}()

	err = remote.Write(ref, img,
		remote.WithAuth(auth),
		remote.WithProgress(progressChannel),
		remote.WithTransport(n.transport),
		remote.WithJobs(1),
		remote.WithContext(ctx))
	if err != nil {
		return "", err
	}
	err = <-errChan
	if err != nil {
		return "", err
	}

	hash, err := img.Digest()
	if err != nil {
		return "", err
	}

	return hash.String(), nil
}
