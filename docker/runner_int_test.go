//go:build integration
// +build integration

package docker_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"

	fn "knative.dev/func"
	"knative.dev/func/docker"
)

const displayEventImg = "gcr.io/knative-releases/knative.dev/eventing/cmd/event_display@sha256:610234e4319b767b187398085971d881956da660a4e0fab65a763e0f81881d82"

func TestRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	prePullTestImages(t)

	// deliberately try to seize 8080
	l, err := net.Listen("tcp", "localhost:8080")
	if err == nil {
		t.Cleanup(func() { _ = l.Close() })
	}

	var out, errOut bytes.Buffer
	runner := docker.NewRunner(true, &out, &errOut)

	j, err := runner.Run(ctx, fn.Function{
		Image: displayEventImg,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(j.Stop)
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
