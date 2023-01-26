//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "knative.dev/func/testing"
)

// TestInvokeFunction is used when testing the 'func invoke' subcommand.
// It responds with a CloudEvent containing an echo of the data received
// If the CloudEvent received has source "func:set" it will update the
// current value of data.
var TestInvokeFunctionImpl = `
package function

import (
  "context"

  ce "github.com/cloudevents/sdk-go/v2/event"
)

var _data = []byte{}
var _type = "text/plain"

func Handle(ctx context.Context, event ce.Event) (*ce.Event, error) {
	if event.Source() == "func:set" {
		_data = event.Data()
		_type = event.DataContentType()
	}
	res := ce.New()
	res.SetSource("func:testInvokeHandler")
	res.SetData(_type, _data)
	return &res, nil
}
`

// TestInvoke ensures that invoking a CloudEvent function succeeds, including
// preserving custom values through the full round-trip.
func TestInvoke(t *testing.T) {
	var (
		root        = "testdata/e2e/testinvoke" // root path for the test function
		bin, prefix = bin()                     // path to test binary and prefix args
		cleanup     = Within(t, root)           // Create and CD to root.
		cwd, _      = os.Getwd()                // the current working directory (absolute)
	)
	defer cleanup()

	run(t, bin, prefix, "create", "--language=go", "--template=cloudevents", cwd)
	set(t, "handle.go", TestInvokeFunctionImpl)
	run(t, bin, prefix, "deploy", "--verbose", "--builder=pack", "--registry", GetRegistry())
	infoOut := run(t, bin, prefix, "info", "--output", "plain")
	run(t, bin, prefix, "invoke", "--verbose", "--content-type=text/plain", "--source=func:set", "--data=TEST")

	// Resolve target service URL from info command stdout
	targetUrl := "http://testinvoke.default.127.0.0.1.sslip.io"
	matches := regexp.MustCompile("Route (http.*)").FindStringSubmatch(infoOut)
	if len(matches) > 1 {
		targetUrl = matches[1]
	}

	// Validate by fetching the contents of the function's data global
	fmt.Println("Validate:")
	req := cloudevents.NewEvent()
	req.SetID("1")
	req.SetSource("func:get")
	req.SetType("func.test")
	c, err := cloudevents.NewClientHTTP(cloudevents.WithTarget(targetUrl))
	if err != nil {
		return
	}
	res, err := c.Request(context.Background(), req)
	if cloudevents.IsUndelivered(err) {
		t.Fatal(err)
	}
	if string(res.Data()) != "TEST" {
		t.Fatalf("expected data 'TEST' got '%v'", string(res.Data()))
	}
	return
}

// bin returns the path to use for the binary plus any leading args that
// should be prepended.
// For example, this will usually either return `/path/to/func` or `kn func`.
// See NewKnFuncShellCli for original source of this logic.
func bin() (path string, args []string) {
	if IsUseKnFunc() {
		return "kn", []string{"func"}
	}
	if path = GetFuncBinaryPath(); path == "" {
		fmt.Fprintf(os.Stderr, "'E2E_FUNC_BIN_PATH' or 'E2E_USE_KN_FUNC' can be used to specify test binary path.")
		return "func", []string{} //default
	}
	return path, []string{}
}

// run the given binary with the given two sets of arguments
// This allows for swappable running between the two forms:
// func [subcommand] [flags]
//
//	and
//
// kn func [subcommand] [flags]
func run(t *testing.T, bin string, prefix []string, suffix ...string) string {
	t.Helper()
	args := append(prefix, suffix...)
	fmt.Printf("%v %v\n", bin, strings.Join(args, " "))

	var stdout bytes.Buffer

	cmd := exec.Command(bin, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return stdout.String()
}

// set the contents of the given file
func set(t *testing.T, path, data string) {
	if err := os.WriteFile(path, []byte(data), os.ModePerm); err != nil {
		t.Fatal(err)
	}
}
