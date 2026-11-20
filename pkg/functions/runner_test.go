package functions

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestGetRunFuncErrors ensures that known runtimes which do not yet
// have their runner implemented return a "not yet available" message, as
// distinct from unrecognized runtimes which state as much.
func TestGetRunFuncErrors(t *testing.T) {
	tests := []struct {
		Runtime    string
		ExpectedIs error
		ExpectedAs any
	}{
		{"", ErrRuntimeRequired, nil},
		{"go", nil, nil},
		{"python", nil, nil},
		{"rust", nil, &ErrRunnerNotImplemented{}},
		{"node", nil, &ErrRunnerNotImplemented{}},
		{"typescript", nil, &ErrRunnerNotImplemented{}},
		{"quarkus", nil, &ErrRunnerNotImplemented{}},
		{"springboot", nil, &ErrRunnerNotImplemented{}},
		{"other", nil, &ErrRuntimeNotRecognized{}},
	}
	for _, test := range tests {
		t.Run(test.Runtime, func(t *testing.T) {

			ctx := t.Context()
			job := Job{Function: Function{Runtime: test.Runtime}}
			_, err := getRunFunc(ctx, &job)

			if test.ExpectedAs != nil && !errors.As(err, test.ExpectedAs) {
				t.Fatalf("did not receive expected error type for %v runtime.", test.Runtime)
			}
			t.Logf("ok: %v", err)
		})
	}
}

// TestBuildRunnerEnv_NoKafkaConfig ensures the function process carries no
// Kafka configuration, even with run.kafka fully configured: a local run
// consumes Kafka through the Kafka runtime started next to the function (see
// startKafkaRuntime), which delivers records to it over HTTP, as the sidecar
// does on the cluster. The FUNC_TRANSPORT=kafka selector of the retired
// in-process consumer must not be set either.
func TestBuildRunnerEnv_NoKafkaConfig(t *testing.T) {
	job := &Job{
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:          "broker:9093",
					Topic:            "my-topic",
					ConsumerGroup:    "my-group",
					SecurityProtocol: "SASL_SSL",
					TLS:              &KafkaTLS{CACert: "/ca.crt"},
					SASL:             &KafkaSASL{User: "u", Password: "p"},
				},
			},
		},
	}
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	inherited := map[string]bool{}
	for _, e := range os.Environ() {
		inherited[e] = true
	}
	for _, e := range env {
		if inherited[e] {
			continue // the user's own environment is passed through as is
		}
		if strings.HasPrefix(e, "KAFKA_") || strings.HasPrefix(e, "FUNC_TRANSPORT=") {
			t.Errorf("unexpected Kafka configuration in the function environment: %v", e)
		}
	}
}

// TestRunFunctionProcess_Stop ensures stopping a host-run job stops the
// function process, and waits for it to exit, without the run's context being
// canceled: func exits without canceling it when a run fails, and a function
// left running would outlive func.
func TestRunFunctionProcess_Stop(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	port, err := freeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	listen := net.JoinHostPort(defaultRunHost, port)

	// This test binary, run as the fake Kafka runtime (see TestMain), stands
	// in for a function: it serves until interrupted.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), fakeKafkaRuntimeEnv+"=ready", "LISTEN_ADDRESS="+listen)
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 5 * time.Second
	job := &Job{Function: Function{Root: t.TempDir()}, Port: port, Errors: make(chan error, 1)}

	runFunctionProcess(job, cmd, cancel)
	if err := waitFor(t.Context(), &Job{Host: defaultRunHost, Port: port}, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	if err := job.Stop(); err != nil {
		t.Fatal(err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("the function process had not exited when the job stop returned")
	}
}
