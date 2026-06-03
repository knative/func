package k8s

import (
	"errors"
	"testing"
	"time"

	"knative.dev/func/pkg/creds"
)

func TestIsECRRegistry(t *testing.T) {
	tests := []struct {
		registry string
		expected bool
	}{
		// ECR Public
		{"public.ecr.aws", true},
		// ECR Private (various regions and partitions)
		{"123456789012.dkr.ecr.us-east-1.amazonaws.com", true},
		{"123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com", true},
		{"123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", true},
		{"123456789012.dkr.ecr.us-east-1.sc2s.sgov.gov", true},
		{"123456789012.dkr.ecr.us-east-1.c2s.ic.gov", true},
		// Non-ECR registries
		{"123456789012.dkr.ecr.us-east-1.example.com", false},
		{"123456789012.dkr.ecr.us-east-1.amazonaws.com.example.com", false},
		{"gcr.io", false},
		{"docker.io", false},
		{"index.docker.io", false},
		{"quay.io", false},
		{"myregistry.azurecr.io", false},
		{"", false},
	}

	for _, tt := range tests {
		name := tt.registry
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			result := isECRRegistry(tt.registry)
			if result != tt.expected {
				t.Errorf("isECRRegistry(%q) = %v; want %v", tt.registry, result, tt.expected)
			}
		})
	}
}

func TestGetECRCredentialLoader(t *testing.T) {
	loader := GetECRCredentialLoader()[0]

	t.Run("non-ECR registry returns ErrCredentialsNotFound", func(t *testing.T) {
		_, err := loader("gcr.io")
		if !errors.Is(err, creds.ErrCredentialsNotFound) {
			t.Errorf("expected ErrCredentialsNotFound, got %v", err)
		}
	})

	t.Run("missing AWS credentials returns ErrCredentialsNotFound", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmp+"/creds")
		t.Setenv("AWS_CONFIG_FILE", tmp+"/config")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_SESSION_TOKEN", "")

		_, err := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")
		if !errors.Is(err, creds.ErrCredentialsNotFound) {
			t.Errorf("expected ErrCredentialsNotFound, got %v", err)
		}
	})

	t.Run("caches failures for 1 minute", func(t *testing.T) {
		loader := GetECRCredentialLoader()[0]
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmp+"/creds")
		t.Setenv("AWS_CONFIG_FILE", tmp+"/config")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_SESSION_TOKEN", "")

		// First call
		start := time.Now()
		_, err1 := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")
		elapsed1 := time.Since(start)

		// Second call should be instant (from cache)
		start2 := time.Now()
		_, err2 := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")
		elapsed2 := time.Since(start2)

		if !errors.Is(err1, creds.ErrCredentialsNotFound) || !errors.Is(err2, creds.ErrCredentialsNotFound) {
			t.Fatal("expected ErrCredentialsNotFound")
		}

		// Cached call should be much faster
		if elapsed2 > 10*time.Millisecond && elapsed1 > 100*time.Millisecond {
			t.Errorf("cache not working: first=%v, second=%v", elapsed1, elapsed2)
		}
	})
}
