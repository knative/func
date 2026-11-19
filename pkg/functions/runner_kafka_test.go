package functions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"knative.dev/func/pkg/kafkaruntime"
)

const (
	// fakeKafkaRuntimeEnv, when set, makes this test binary act as a fake Kafka
	// runtime instead of running tests (see TestMain); its value selects the
	// fake's behavior (see fakeKafkaRuntime).
	fakeKafkaRuntimeEnv = "FUNC_TEST_FAKE_KAFKA_RUNTIME"

	// fakeKafkaRuntimeEnvFile names the file the fake records its environment in.
	fakeKafkaRuntimeEnvFile = "FUNC_TEST_FAKE_KAFKA_RUNTIME_ENV_FILE"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeKafkaRuntimeEnv); mode != "" {
		os.Exit(fakeKafkaRuntime(mode))
	}
	os.Exit(m.Run())
}

// fakeKafkaRuntime stands in for the Kafka runtime. It records its environment
// and then, per mode:
//
//	ready    serves readiness 200 until interrupted
//	unready  serves readiness 503 until interrupted
//	exit     exits 3 at once, as the runtime does when it cannot reach Kafka
//	crash    serves readiness 200, then exits 2 shortly after first probed
func fakeKafkaRuntime(mode string) int {
	if f := os.Getenv(fakeKafkaRuntimeEnvFile); f != "" {
		if err := os.WriteFile(f, []byte(strings.Join(os.Environ(), "\n")), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Println("fake runtime starting")
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "fake runtime: cannot reach brokers")
		return 3
	}

	l, err := net.Listen("tcp", os.Getenv("LISTEN_ADDRESS"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var crash sync.Once
	http.HandleFunc("/health/readiness", func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "unready":
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		case "crash":
			crash.Do(func() { time.AfterFunc(200*time.Millisecond, func() { os.Exit(2) }) })
		}
		w.WriteHeader(http.StatusOK)
	})
	go func() { _ = http.Serve(l, nil) }()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
	return 0
}

// useFakeKafkaRuntime makes startKafkaRuntime run this test binary as a fake
// Kafka runtime in the given mode, and returns the path of the file in which
// the fake records its environment.
func useFakeKafkaRuntime(t *testing.T, mode string) (envFile string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stubKafkaRuntimeBinary(t, func() (string, error) { return exe, nil })
	envFile = filepath.Join(t.TempDir(), "env")
	t.Setenv(fakeKafkaRuntimeEnv, mode)
	t.Setenv(fakeKafkaRuntimeEnvFile, envFile)
	return envFile
}

func stubKafkaRuntimeBinary(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := kafkaRuntimeBinary
	kafkaRuntimeBinary = fn
	t.Cleanup(func() { kafkaRuntimeBinary = orig })
}

// newKafkaJob returns a job for a function consuming per k, as a runner would
// return it, and a flag set once the job's (function's) stop is invoked.
func newKafkaJob(t *testing.T, k *KafkaConfig) (*Job, *atomic.Bool) {
	t.Helper()
	stopped := &atomic.Bool{}
	job := &Job{
		Function: Function{Root: t.TempDir(), Runtime: "go", Run: RunSpec{Kafka: k}},
		Host:     "127.0.0.1",
		Port:     "18080",
		Errors:   make(chan error, 1),
		onStop:   func() error { stopped.Store(true); return nil },
	}
	return job, stopped
}

func kafkaConfig() *KafkaConfig {
	return &KafkaConfig{Brokers: "broker:9092", Topic: "my-topic", ConsumerGroup: "my-group"}
}

// readEnvFile returns the environment recorded by the fake runtime.
func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, e := range strings.Split(string(data), "\n") {
		k, v, _ := strings.Cut(e, "=")
		env[k] = v
	}
	return env
}

// TestStartKafkaRuntime_Ready ensures the runtime is started configured to
// deliver to the function, that the job then stops it ahead of the function,
// and that a requested stop is not reported as a failure.
func TestStartKafkaRuntime_Ready(t *testing.T) {
	envFile := useFakeKafkaRuntime(t, "ready")
	// Configuration in the user's shell must not reach the runtime: like the
	// sidecar, it is configured by run.kafka alone.
	t.Setenv("KAFKA_SASL_PASSWORD", "from-the-shell")
	t.Setenv("FUNCTION_DELIVERY_RETRY", "7")

	job, functionStopped := newKafkaJob(t, kafkaConfig())
	if err := startKafkaRuntime(t.Context(), job, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	env := readEnvFile(t, envFile)
	for k, v := range map[string]string{
		"FUNCTION_TARGET":      "http://127.0.0.1:18080/",
		"KAFKA_BROKERS":        "broker:9092",
		"KAFKA_TOPIC":          "my-topic",
		"KAFKA_CONSUMER_GROUP": "my-group",
	} {
		if env[k] != v {
			t.Errorf("runtime env %v=%q, want %q", k, env[k], v)
		}
	}
	for _, k := range []string{"KAFKA_SASL_PASSWORD", "FUNCTION_DELIVERY_RETRY"} {
		if v, ok := env[k]; ok {
			t.Errorf("runtime env has %v=%q inherited from the shell", k, v)
		}
	}
	host, port, err := net.SplitHostPort(env["LISTEN_ADDRESS"])
	if err != nil || host != "127.0.0.1" || port == job.Port {
		t.Fatalf("runtime LISTEN_ADDRESS=%q, want a loopback address on a port other than the function's", env["LISTEN_ADDRESS"])
	}

	if err := job.Stop(); err != nil {
		t.Fatal(err)
	}
	if !functionStopped.Load() {
		t.Error("stopping the job did not stop the function")
	}
	// The runtime has exited by the time the job is stopped.
	if res, err := http.Get("http://" + env["LISTEN_ADDRESS"] + readinessEndpoint); err == nil {
		res.Body.Close()
		t.Error("the Kafka runtime is still serving after the job was stopped")
	}
	select {
	case err := <-job.Errors:
		t.Fatalf("a requested stop was reported as a runtime error: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestStartKafkaRuntime_ExitBeforeReady ensures a runtime that exits during
// startup (an unreachable broker, bad credentials) fails the run at once,
// without waiting out the start timeout.
func TestStartKafkaRuntime_ExitBeforeReady(t *testing.T) {
	useFakeKafkaRuntime(t, "exit")
	job, functionStopped := newKafkaJob(t, kafkaConfig())

	start := time.Now()
	err := startKafkaRuntime(t.Context(), job, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "exited before becoming ready") || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("expected an early-exit error carrying the exit status, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("the early exit was detected only after %v", elapsed)
	}
	if functionStopped.Load() {
		t.Error("startKafkaRuntime stopped the function; that is its caller's decision")
	}
}

func TestStartKafkaRuntime_NotReady(t *testing.T) {
	useFakeKafkaRuntime(t, "unready")
	job, _ := newKafkaJob(t, kafkaConfig())

	err := startKafkaRuntime(t.Context(), job, 1500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not become ready within 1.5s") {
		t.Fatalf("expected a readiness timeout error, got %v", err)
	}
}

// TestStartKafkaRuntime_ExitAfterReady ensures a runtime exiting mid-run is
// reported on the job's error channel, ending the run as a function crash does.
func TestStartKafkaRuntime_ExitAfterReady(t *testing.T) {
	useFakeKafkaRuntime(t, "crash")
	job, _ := newKafkaJob(t, kafkaConfig())
	if err := startKafkaRuntime(t.Context(), job, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Stop() }()

	select {
	case err := <-job.Errors:
		if !strings.Contains(err.Error(), "exited unexpectedly") || !strings.Contains(err.Error(), "exit status 2") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the runtime's exit was not reported")
	}
}

// TestStartKafkaRuntime_RunCanceled ensures the runtime stops with the run
// (Ctrl-C) and that this is not reported as a runtime failure.
func TestStartKafkaRuntime_RunCanceled(t *testing.T) {
	useFakeKafkaRuntime(t, "ready")
	job, _ := newKafkaJob(t, kafkaConfig())
	ctx, cancel := context.WithCancel(t.Context())
	if err := startKafkaRuntime(ctx, job, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := job.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-job.Errors:
		t.Fatalf("canceling the run was reported as a runtime error: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestStartKafkaRuntime_NotConfigured(t *testing.T) {
	stubKafkaRuntimeBinary(t, func() (string, error) {
		t.Fatal("the Kafka runtime was resolved for a function not consuming from Kafka")
		return "", nil
	})
	for _, k := range []*KafkaConfig{
		nil,
		{Brokers: "broker:9092", ConsumerGroup: "my-group"}, // no topic
	} {
		job, _ := newKafkaJob(t, k)
		if err := startKafkaRuntime(t.Context(), job, time.Second); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStartKafkaRuntime_Unavailable ensures a func build without a Kafka
// runtime for this platform still runs the function, and says why it will
// not receive records rather than silently consuming nothing.
func TestStartKafkaRuntime_Unavailable(t *testing.T) {
	stubKafkaRuntimeBinary(t, func() (string, error) { return "", kafkaruntime.ErrUnavailable })
	job, _ := newKafkaJob(t, kafkaConfig())

	stderr := captureStderr(t, func() {
		if err := startKafkaRuntime(t.Context(), job, time.Second); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"Warning:", `"my-topic"`, kafkaruntime.BinaryEnv} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected the warning to contain %q, got %q", want, stderr)
		}
	}
}

// TestStartKafkaRuntime_ConfigErrors ensures invalid configuration fails the
// run before any runtime is started.
func TestStartKafkaRuntime_ConfigErrors(t *testing.T) {
	stubKafkaRuntimeBinary(t, func() (string, error) {
		t.Fatal("the Kafka runtime was resolved despite invalid configuration")
		return "", nil
	})

	k := kafkaConfig()
	k.Delivery = &DeliverySpec{BackoffDelay: "soon"}
	job, _ := newKafkaJob(t, k)
	if err := startKafkaRuntime(t.Context(), job, time.Second); err == nil || !strings.Contains(err.Error(), "run.kafka.delivery.backoffDelay") {
		t.Fatalf("expected a delivery validation error, got %v", err)
	}

	k = kafkaConfig()
	k.SecurityProtocol = "SASL_PLAINTEXT"
	k.SASL = &KafkaSASL{User: "u", Password: "{{ env:FUNC_TEST_KAFKA_UNSET }}"}
	job, _ = newKafkaJob(t, k)
	if err := startKafkaRuntime(t.Context(), job, time.Second); err == nil || !strings.Contains(err.Error(), "FUNC_TEST_KAFKA_UNSET") {
		t.Fatalf("expected an unresolved credential error, got %v", err)
	}
}

// TestClientRun_KafkaRuntime ensures Client.Run starts the Kafka runtime for
// whichever runner runs the function, and stops the function when the runtime
// cannot start, rather than leaving it running without Kafka delivery.
func TestClientRun_KafkaRuntime(t *testing.T) {
	f := Function{Root: t.TempDir(), Runtime: "go", Created: time.Now(), Run: RunSpec{Kafka: kafkaConfig()}}

	t.Run("ready", func(t *testing.T) {
		useFakeKafkaRuntime(t, "ready")
		job, functionStopped := newKafkaJob(t, f.Run.Kafka)
		client := New(WithRunner(stubRunner{job}))

		got, err := client.Run(t.Context(), f)
		if err != nil {
			t.Fatal(err)
		}
		if err := got.Stop(); err != nil {
			t.Fatal(err)
		}
		if !functionStopped.Load() {
			t.Error("stopping the job did not stop the function")
		}
	})

	t.Run("runtime fails", func(t *testing.T) {
		useFakeKafkaRuntime(t, "exit")
		job, functionStopped := newKafkaJob(t, f.Run.Kafka)
		client := New(WithRunner(stubRunner{job}))

		if _, err := client.Run(t.Context(), f); err == nil {
			t.Fatal("expected Run to fail when the Kafka runtime cannot start")
		}
		if !functionStopped.Load() {
			t.Error("the function was left running after the Kafka runtime failed to start")
		}
	})
}

// stubRunner is a Runner returning a given, already running, job.
type stubRunner struct{ job *Job }

func (r stubRunner) Run(context.Context, Function, string, time.Duration) (*Job, error) {
	return r.job, nil
}

func TestKafkaRuntimeEnv(t *testing.T) {
	retry := 0
	k := &KafkaConfig{
		Brokers:          "b1:9093,b2:9093",
		Topic:            "my-topic",
		ConsumerGroup:    "my-group",
		SecurityProtocol: "SASL_SSL",
		TLS:              &KafkaTLS{CACert: "/ca.crt", ClientCert: "/client.crt", ClientKey: "/client.key", SkipVerify: true},
		SASL:             &KafkaSASL{Mechanism: "SCRAM-SHA-512", User: "my-user", Password: "my-pass"},
		Delivery:         &DeliverySpec{Retry: &retry, BackoffPolicy: "linear", BackoffDelay: "200ms", BackoffMaxDelay: "5s", Timeout: "10s"},
	}
	env, warnings, err := kafkaRuntimeEnv(k)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"KAFKA_BROKERS=b1:9093,b2:9093",
		"KAFKA_TOPIC=my-topic",
		"KAFKA_CONSUMER_GROUP=my-group",
		"KAFKA_SECURITY_PROTOCOL=SASL_SSL",
		"KAFKA_TLS_CA_CERT=/ca.crt",
		"KAFKA_TLS_CLIENT_CERT=/client.crt",
		"KAFKA_TLS_CLIENT_KEY=/client.key",
		"KAFKA_TLS_SKIP_VERIFY=true",
		"KAFKA_SASL_MECHANISM=SCRAM-SHA-512",
		"KAFKA_SASL_USER=my-user",
		"KAFKA_SASL_PASSWORD=my-pass",
		"FUNCTION_DELIVERY_RETRY=0", // an explicit 0 (a single attempt) is passed on
		"FUNCTION_DELIVERY_BACKOFF_POLICY=linear",
		"FUNCTION_DELIVERY_BACKOFF_DELAY=200ms",
		"FUNCTION_DELIVERY_BACKOFF_MAX_DELAY=5s",
		"FUNCTION_REQUEST_TIMEOUT=10s",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("unexpected runtime env\n got: %q\nwant: %q", env, want)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

// TestKafkaRuntimeEnv_Defaults ensures optional settings left unset are not
// passed on, so the runtime applies its own defaults as it does on the
// cluster, except for the two the cluster sidecar also always sets.
func TestKafkaRuntimeEnv_Defaults(t *testing.T) {
	env, _, err := kafkaRuntimeEnv(kafkaConfig())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"KAFKA_BROKERS=broker:9092", "KAFKA_TOPIC=my-topic", "KAFKA_CONSUMER_GROUP=my-group"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("got %q, want %q", env, want)
	}

	// An empty TLS block still sets skip-verify (false), and an unset SASL
	// mechanism resolves to PLAIN, matching the scaler's EffectiveMechanism.
	k := kafkaConfig()
	k.TLS = &KafkaTLS{}
	k.SASL = &KafkaSASL{User: "u", Password: "p"}
	k.Delivery = &DeliverySpec{}
	env, _, err = kafkaRuntimeEnv(k)
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, "KAFKA_TLS_SKIP_VERIFY=false", "KAFKA_SASL_MECHANISM=PLAIN", "KAFKA_SASL_USER=u", "KAFKA_SASL_PASSWORD=p")
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("got %q, want %q", env, want)
	}
}

func TestKafkaRuntimeEnv_Credentials(t *testing.T) {
	t.Setenv("FUNC_TEST_KAFKA_PASSWORD", "s3cret")
	k := kafkaConfig()
	k.SASL = &KafkaSASL{User: "{{ secret:kafka-auth:user }}", Password: "{{ env:FUNC_TEST_KAFKA_PASSWORD }}"}

	env, warnings, err := kafkaRuntimeEnv(k)
	if err != nil {
		t.Fatal(err)
	}
	has := func(e string) bool {
		for _, x := range env {
			if x == e {
				return true
			}
		}
		return false
	}
	// A local environment reference is resolved.
	if !has("KAFKA_SASL_PASSWORD=s3cret") {
		t.Errorf("expected the password resolved from the local environment, got %q", env)
	}
	// A cluster reference is passed on verbatim, with a warning naming it.
	if !has("KAFKA_SASL_USER={{ secret:kafka-auth:user }}") {
		t.Errorf("expected the cluster reference passed on verbatim, got %q", env)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "run.kafka.sasl.user") || !strings.Contains(warnings[0], "{{ secret:kafka-auth:user }}") {
		t.Errorf("expected one warning naming run.kafka.sasl.user and its reference, got %q", warnings)
	}
	for _, w := range warnings {
		if strings.Contains(w, "s3cret") {
			t.Errorf("a warning reveals a credential: %q", w)
		}
	}

	// A local environment reference to an unset variable is an error.
	k.SASL.Password = "{{ env:FUNC_TEST_KAFKA_UNSET }}"
	if _, _, err := kafkaRuntimeEnv(k); err == nil || !strings.Contains(err.Error(), "run.kafka.sasl.password") {
		t.Fatalf("expected an error naming run.kafka.sasl.password, got %v", err)
	}
}

func TestWithoutKafkaRuntimeEnv(t *testing.T) {
	got := withoutKafkaRuntimeEnv([]string{
		"PATH=/bin",
		"KAFKA_BROKERS=b:9092",
		"kafka_topic=t",
		"KAFKA_SASL_PASSWORD=p",
		"FUNCTION_TARGET=http://x/",
		"FUNCTION_DELIVERY_RETRY=1",
		"FUNCTION_REQUEST_TIMEOUT=1s",
		"LISTEN_ADDRESS=:8081",
		"FUNCTION_NAME=kept",
		"MY_KAFKA_SETTING=kept",
		"FUNC_VERBOSE=true",
	})
	want := []string{"PATH=/bin", "FUNCTION_NAME=kept", "MY_KAFKA_SETTING=kept", "FUNC_VERBOSE=true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestKafkaFunctionTarget(t *testing.T) {
	for _, tt := range []struct{ host, port, want string }{
		{"127.0.0.1", "8080", "http://127.0.0.1:8080/"},
		{"", "8080", "http://127.0.0.1:8080/"},
		{"0.0.0.0", "9000", "http://127.0.0.1:9000/"},
		{"::", "9000", "http://[::1]:9000/"},
		{"::1", "9000", "http://[::1]:9000/"},
		{"192.168.1.5", "8080", "http://192.168.1.5:8080/"},
		{"localhost", "8080", "http://localhost:8080/"},
	} {
		if got := kafkaFunctionTarget(tt.host, tt.port); got != tt.want {
			t.Errorf("kafkaFunctionTarget(%q, %q) = %q, want %q", tt.host, tt.port, got, tt.want)
		}
	}
}

// writes records each Write made to it.
type writes []string

func (w *writes) Write(b []byte) (int, error) {
	*w = append(*w, string(b))
	return len(b), nil
}

func TestLinePrefixWriter(t *testing.T) {
	var out writes
	w := newLinePrefixWriter(&out, "[kafka] ")

	for _, s := range []string{"one\ntw", "o\n", "three\nfour\n", "five"} {
		if n, err := w.Write([]byte(s)); err != nil || n != len(s) {
			t.Fatalf("Write(%q) = %v, %v", s, n, err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil { // nothing left to flush
		t.Fatal(err)
	}
	// Each line is written whole, in a single write.
	want := writes{"[kafka] one\n", "[kafka] two\n", "[kafka] three\n", "[kafka] four\n", "[kafka] five\n"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("got %q, want %q", out, want)
	}

	// An unterminated line is written out once it reaches the cap.
	out = nil
	if _, err := w.Write([]byte(strings.Repeat("x", kafkaRuntimeMaxLine))); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out[0]) != len("[kafka] ")+kafkaRuntimeMaxLine+1 {
		t.Fatalf("expected the capped line to be written out, got %d writes", len(out))
	}
}

func TestLinePrefixWriter_Error(t *testing.T) {
	w := newLinePrefixWriter(failingWriter{}, "[kafka] ")
	if _, err := w.Write([]byte("line\n")); err == nil {
		t.Fatal("expected the underlying write error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken") }

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	w.Close()
	return <-out
}
