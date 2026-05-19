package mock

import (
	"context"
	"testing"
)

func TestMCPServer_Start(t *testing.T) {
	s := NewMCPServer()
	if s.StartInvoked {
		t.Fatal("StartInvoked should be false before calling Start")
	}

	err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.StartInvoked {
		t.Fatal("StartInvoked should be true after calling Start")
	}
}
