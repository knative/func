//go:build integration
// +build integration

package docker_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

const displayEventImg = "gcr.io/knative-releases/knative.dev/eventing/cmd/event_display@sha256:610234e4319b767b187398085971d881956da660a4e0fab65a763e0f81881d82"

func TestRun(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	prePullTestImages(t)

	// No need to check for port 8080 since the runner should automatically
	// choose an open port, with 8080 only being the preferred first choice.

	// Initialize a new function (creates all artifacts on disk necessary
	// to perform actions such as running)
	f := fn.Function{Runtime: "go", Root: root, Image: displayEventImg}

	client := fn.New()
	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(ctx, f)

	// Run the function using a docker runner
	var out, errOut bytes.Buffer
	runner := docker.NewRunner(true, &out, &errOut)
	j, err := runner.Run(ctx, f, fn.DefaultStartTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Stop() })
	time.Sleep(time.Second * 5)

	var (
		id  = "runner-test-id"
		src = "runner-test-src"
		typ = "runner-test-type"
	)

	event := cloudevents.NewEvent()
	event.SetID(id)
	event.SetSource(src)
	event.SetType(typ)

	c, err := cloudevents.NewClientHTTP(cloudevents.WithTarget("http://localhost:" + j.Port))
	if err != nil {
		t.Fatal(err)
	}

	var httpErr *http.Result
	res := c.Send(ctx, event)
	if ok := errors.As(res, &httpErr); ok {
		if httpErr.StatusCode < 200 || httpErr.StatusCode > 299 {
			t.Fatal("non 2XX code")
		}
	} else {
		t.Error("expected http.Result type")
	}
	time.Sleep(time.Second * 5)

	t.Log("out: ", out.String())
	t.Log("errOut: ", errOut.String())

	outStr := out.String()

	if !(strings.Contains(outStr, id) && strings.Contains(outStr, src) && strings.Contains(outStr, typ)) {
		t.Error("output doesn't contain invocation info")
	}
}

func prePullTestImages(t *testing.T) {
	t.Helper()
	c, _, err := docker.NewClient(dockerClient.DefaultDockerHost)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.ImagePull(context.Background(), displayEventImg, types.ImagePullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp)
}
