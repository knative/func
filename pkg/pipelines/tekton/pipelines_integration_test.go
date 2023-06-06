//go:build integration
// +build integration

package tekton_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/random"
)

func TestOnClusterBuild(t *testing.T) {
	checkTestEnabled(t)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ns := "default"

	credentialsProvider := func(ctx context.Context, image string) (docker.Credentials, error) {
		return docker.Credentials{
			Username: "",
			Password: "",
		}, nil
	}

	tests := []struct {
		Builder string
	}{
		{Builder: "s2i"},
		{Builder: "pack"},
	}

	for _, test := range tests {
		t.Run(test.Builder, func(t *testing.T) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			urlChan := make(chan string, 1)

			pp := tekton.NewPipelinesProvider(
				tekton.WithCredentialsProvider(credentialsProvider),
				tekton.WithNamespace(ns),
				tekton.WithProgressListener(pl{urlChan: urlChan}))

			f := createSimpleGoProject(t, ns)
			f.Build.Builder = test.Builder

			go func() {
				err := pp.Run(ctx, f)
				if err != nil {
					t.Error(err)
					cancel()
				}
			}()

			select {
			case u := <-urlChan:
				resp, err := http.Get(u)
				if err != nil {
					t.Error(err)
					return
				}
				_ = resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Error("bad HTTP response code")
					return
				}
				t.Log("call to knative service successful")
			case <-time.After(time.Minute * 10):
				t.Error("timeout while waiting for service to start")
			case <-ctx.Done():
				t.Error("cancelled")
			}
		})
	}
}

func checkTestEnabled(t *testing.T) {
	val := os.Getenv("TEKTON_TESTS_ENABLED")
	enabled, _ := strconv.ParseBool(val)
	if !enabled {
		t.Skip("tekton tests are not enabled")
	}
}

type pl struct {
	urlChan chan<- string
}

func (p pl) log(args ...any) {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		prefix := fmt.Sprintf("%s:%d", filepath.Base(file), line)
		args = append([]any{prefix}, args...)
	}
	fmt.Fprintln(os.Stderr, args...)
}

func (p pl) SetTotal(i int) {
	p.log("ProgressListener::SetTotal: ", i)
}

func (p pl) Increment(message string) {
	p.log("ProgressListener::Increment: ", message)
	if strings.Contains(message, "URL:") {
		parts := strings.Split(message, "URL:")
		if len(parts) < 2 {
			p.log("bad output message: %q", message)
			return
		}
		u := strings.TrimSpace(parts[1])
		p.urlChan <- u
	}
}

func (p pl) Complete(message string) {
	p.log("ProgressListener::Complete: ", message)
}

func (p pl) Stopping() {
	p.log("ProgressListener::Stopping")
}

func (p pl) Done() {
	p.log("ProgressListener::Done")
}

func createSimpleGoProject(t *testing.T, ns string) fn.Function {
	var err error

	funcName := "fn-" + strings.ToLower(random.AlphaString(5))

	projDir := filepath.Join(t.TempDir(), funcName)
	err = os.Mkdir(projDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projDir, "main.go"), []byte(simpleGOSvc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module web\n\ngo 1.20\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	f := fn.Function{
		Root:     projDir,
		Name:     funcName,
		Runtime:  "none",
		Template: "none",
		Image:    "registry.default.svc.cluster.local:5000/" + funcName,
		Created:  time.Now(),
		Invoke:   "none",
		Build: fn.BuildSpec{
			BuilderImages: map[string]string{
				"pack": "docker.io/paketobuildpacks/builder:base",
				"s2i":  "registry.access.redhat.com/ubi8/go-toolset",
			},
		},
		Deploy: fn.DeploySpec{
			Namespace: ns,
		},
	}
	f = fn.NewFunctionWith(f)
	err = f.Write()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const simpleGOSvc = `package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sigs := make(chan os.Signal, 5)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	s := http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			resp.Header().Add("Content-Type", "text/plain")
			resp.WriteHeader(200)
			_, _ = resp.Write([]byte("OK"))
		}),
	}
	go func() {
		<-sigs
		_ = s.Shutdown(context.Background())
	}()
	port := "8080"
	if p, ok := os.LookupEnv("PORT"); ok {
		port = p
	}
	l, err := net.Listen("tcp4", ":"+port)
	if err != nil {
		panic(err)
	}
	_ = s.Serve(l)
}
`
