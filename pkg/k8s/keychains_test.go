package k8s

import (
	"context"
	"testing"
)

func TestACRCredentialLoader_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loader := GetACRCredentialLoader()[0]

	registry := "example.azurecr.io"

	_, err := loader(ctx, registry)
	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
	t.Logf("Successfully caught cancellation error: %v", err)
}
