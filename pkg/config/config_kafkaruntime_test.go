package config_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/kafkaruntime"
)

// TestDir_KafkaRuntime ensures the Kafka runtime is extracted under func's
// config directory. kafkaruntime derives that directory itself, since it cannot
// import this package (which depends on it through pkg/k8s), so this keeps the
// two derivations in step.
func TestDir_KafkaRuntime(t *testing.T) {
	check := func(t *testing.T) {
		t.Helper()
		want := filepath.Join(config.Dir(), "kafka-runtime", kafkaruntime.Version)
		if got := kafkaruntime.Dir(); got != want {
			t.Fatalf("kafkaruntime.Dir() = %q, want %q (under config.Dir())", got, want)
		}
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Run("home", check)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Run("XDG_CONFIG_HOME", check)
}
