package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	bosonFunc "github.com/boson-project/func"

	"io"
	"os"
	"regexp"
	"strings"

	"github.com/containers/image/v5/pkg/docker/config"
	containersTypes "github.com/containers/image/v5/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
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

	var registry string
	parts := strings.Split(f.Image, "/")
	switch len(parts) {
	case 2:
		registry = "docker.io"
	case 3:
		registry = parts[0]
	default:
		return "", errors.Errorf("failed to parse image name: %q", f.Image)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", errors.Wrap(err, "failed to create docker api client")
	}

	credentials, err := config.GetCredentials(nil, registry)
	if err != nil {
		return "", errors.Wrap(err, "failed to get credentials")
	}

	var opts types.ImagePushOptions

	if credentials == (containersTypes.DockerAuthConfig{}) {

		fmt.Print("Username: ")
		username, err := getUserName(ctx)
		if err != nil {
			return "", err
		}

		fmt.Print("Password: ")
		bytePassword, err := getPassword(ctx)
		if err != nil {
			return "", err
		}
		password := string(bytePassword)

		credentials.Username, credentials.Password = username, password
	}

	b, err := json.Marshal(&credentials)
	if err != nil {
		return "", err
	}
	opts.RegistryAuth = base64.StdEncoding.EncodeToString(b)

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

func getPassword(ctx context.Context) ([]byte, error) {
	ch := make(chan struct {
		p []byte
		e error
	})

	go func() {
		pass, err := terminal.ReadPassword(0)
		ch <- struct {
			p []byte
			e error
		}{p: pass, e: err}
	}()

	select {
	case res := <-ch:
		return res.p, res.e
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func getUserName(ctx context.Context) (string, error) {
	ch := make(chan struct {
		u string
		e error
	})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		username, err := reader.ReadString('\n')
		if err != nil {
			ch <- struct {
				u string
				e error
			}{u: "", e: err}
		}
		ch <- struct {
			u string
			e error
		}{u: strings.TrimRight(username, "\n"), e: nil}
	}()

	select {
	case res := <-ch:
		return res.u, res.e
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
