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
	"github.com/docker/docker/api/types/image"
	dockerClient "github.com/docker/docker/client"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

const displayEventImg = "gcr.io/knative-releases/knative.dev/eventing/cmd/event_display@sha256:610234e4319b767b187398085971d881956da660a4e0fab65a763e0f81881d82"

// public image from repo (author: github.com/gauron99)
const testImageWithDigest = "index.docker.io/4141gauron3268/teste-builder@sha256:4cf9eddf34f14cc274364a4ae60274301385d470de1fb91cbc6fec1227daa739"

func TestRun(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)
	image := displayEventImg
	prePullTestImages(t, image)

	// No need to check for port 8080 since the runner should automatically
	// choose an open port, with 8080 only being the preferred first choice.

	// Initialize a new function (creates all artifacts on disk necessary
	// to perform actions such as running)
	f := fn.Function{Runtime: "go", Root: root, Image: image}

	client := fn.New()
	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

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

// TestRunDigested ensures that passing a digested image to the runner will deploy
// that image instead of the previously built one. This test is depended on the
// specific image since its verifying the function's output.
func TestRunDigested(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	// TODO: gauron99 - if image-digest-on-build is implemented, rework this
	// to fit this schema -- build image (get digest) then run from temporary dir
	// such that its .func stamp is not considered. All of this to remove the
	// external pre-built container dependency
	image := testImageWithDigest
	prePullTestImages(t, image)

	f := fn.Function{Runtime: "go", Root: root, Registry: "docker.io/jdoe"}

	client := fn.New()
	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// prebuild default image
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// simulate passing image from --image flag since client.Run just sets
	// a timeout and simply calls runner.Run.
	f.Build.Image = image

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
		id  = "runner-my-id"
		src = "runner-my-src"
		typ = "runner-my-type"
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

	if !(strings.Contains(outStr, "testing the waters - serverside")) {
		t.Error("output doesn't contain expected text")
	}
}

func prePullTestImages(t *testing.T, img string) {
	t.Helper()
	c, _, err := docker.NewClient(dockerClient.DefaultDockerHost)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.ImagePull(context.Background(), img, image.PullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp)
}
