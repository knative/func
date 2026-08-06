package k8s

import (
	"errors"
	"testing"

	"knative.dev/func/pkg/creds"
)

func TestGetECRCredentialLoader(t *testing.T) {
	loaders := GetECRCredentialLoader()
	if len(loaders) == 0 {
		t.Fatal("expected at least one ECR credential loader")
	}
	loader := loaders[0]

	t.Run("non-ECR registry returns ErrCredentialsNotFound error", func(t *testing.T) {
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
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_ROLE_ARN", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

		_, err := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")
		if !errors.Is(err, creds.ErrCredentialsNotFound) {
			t.Errorf("expected ErrCredentialsNotFound, got %v", err)
		}
	})

	t.Run("caches failures for 1 minute", func(t *testing.T) {
		loaders := GetECRCredentialLoader()
		if len(loaders) == 0 {
			t.Fatal("expected at least one ECR credential loader")
		}
		loader := loaders[0]
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmp+"/creds")
		t.Setenv("AWS_CONFIG_FILE", tmp+"/config")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_SESSION_TOKEN", "")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_ROLE_ARN", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

		// First call
		_, err1 := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")

		// Second call should be from cache
		_, err2 := loader("123456789012.dkr.ecr.us-east-1.amazonaws.com")

		if !errors.Is(err1, creds.ErrCredentialsNotFound) || !errors.Is(err2, creds.ErrCredentialsNotFound) {
			t.Fatal("expected ErrCredentialsNotFound")
		}
	})
}
