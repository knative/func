package k8s

import (
	"errors"
	"testing"

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
		t.Run(tt.registry, func(t *testing.T) {
			result := isECRRegistry(tt.registry)
			if result != tt.expected {
				t.Errorf("isECRRegistry(%q) = %v; want %v", tt.registry, result, tt.expected)
			}
		})
	}
}

func TestGetECRCredentialLoader(t *testing.T) {
	loaders := GetECRCredentialLoader()
	if len(loaders) != 1 {
		t.Fatalf("expected 1 loader callback, got %d", len(loaders))
	}

	loader := loaders[0]

	t.Run("returns ErrCredentialsNotFound for non-ECR registry", func(t *testing.T) {
		_, err := loader("gcr.io")
		if !errors.Is(err, creds.ErrCredentialsNotFound) {
			t.Errorf("expected ErrCredentialsNotFound for non-ECR registry, got %v", err)
		}
	})

	t.Run("returns ErrCredentialsNotFound for ECR registry when AWS credentials are not configured", func(t *testing.T) {
		tmp := t.TempDir()
		// Make the test deterministic by clearing common ambient credential sources.
		t.Setenv("HOME", tmp)
		t.Setenv("USERPROFILE", tmp)
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_SESSION_TOKEN", "")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmp+"/credentials")
		t.Setenv("AWS_CONFIG_FILE", tmp+"/config")
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_ROLE_ARN", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		_, err := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")
		if !errors.Is(err, creds.ErrCredentialsNotFound) {
			t.Errorf("expected ErrCredentialsNotFound when AWS credentials are not configured, got %v", err)
		}
	})
}
