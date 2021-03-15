package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	bosonFunc "github.com/boson-project/func"
	"github.com/containers/image/v5/pkg/docker/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"io"
	"os"
	"regexp"
	"strings"
)

// Pusher of images from local to remote registry.
type Pusher struct {
	// Verbose logging.
	Verbose bool
}

// NewPusher creates an instance of a docker-based image pusher.
func NewPusher() *Pusher {
	return &Pusher{}
}

// Push the image of the Function.
func (n *Pusher) Push(ctx context.Context, f bosonFunc.Function) (digest string, err error) {

	if f.Image == "" {
		return "", errors.New("Function has no associated image.  Has it been built?")
	}

	parts := strings.Split(f.Image, "/")
	if len(parts) < 1 {
		return "", errors.Errorf("invalid image name: %q", f.Image)
	}
	registry := parts[0]

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", errors.Wrap(err, "failed to create docker api client")
	}

	credentials, err := config.GetAllCredentials(nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to get credentials")
	}

	var opts types.ImagePushOptions

	c := credentials[registry]
	opts.RegistryAuth = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"username":"%s","password":"%s"}`, c.Username, c.Password)))

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
		if li.Id != "" {
			fmt.Fprintf(output, "%s: ", li.Id)
		}
		fmt.Fprintln(output, li.Status)
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

type logItem struct {
	Status string `json:"status"`
	Id     string `json:"id"`
}
