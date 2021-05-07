package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	fn "github.com/boson-project/func"
)

type Opt func(*Pusher) error

type Credentials struct {
	Username string
	Password string
}

type CredentialsProvider func(ctx context.Context, registry string) (Credentials, error)

// Pusher of images from local to remote registry.
type Pusher struct {
	// Verbose logging.
	Verbose             bool
	credentialsProvider CredentialsProvider
}

func WithCredentialsProvider(cp CredentialsProvider) Opt {
	return func(p *Pusher) error {
		p.credentialsProvider = cp
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
	}
	for _, opt := range opts {
		err := opt(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Push the image of the Function.
func (n *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {

	if f.Image == "" {
		return "", errors.New("Function has no associated image.  Has it been built?")
	}

	var registry string
	parts := strings.Split(f.Image, "/")
	switch len(parts) {
	case 2:
		registry = fn.DefaultRegistry
	case 3:
		registry = parts[0]
	default:
		return "", errors.Errorf("failed to parse image name: %q", f.Image)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", errors.Wrap(err, "failed to create docker api client")
	}

	credentials, err := n.credentialsProvider(ctx, registry)
	if err != nil {
		return "", errors.Wrap(err, "failed to get credentials")
	}

	b, err := json.Marshal(&credentials)
	if err != nil {
		return "", err
	}

	opts := types.ImagePushOptions{RegistryAuth: base64.StdEncoding.EncodeToString(b)}

	r, err := cli.ImagePush(ctx, f.Image, opts)
	if err != nil {
		return "", errors.Wrap(err, "failed to push the image")
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
