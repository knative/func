package mock

import (
	"context"
)

type MCPServer struct {
	StartInvoked bool
	StartFn      func(context.Context, bool) error
}

func NewMCPServer() *MCPServer {
	return &MCPServer{
		StartFn: func(context.Context, bool) error { return nil },
	}
}

func (s *MCPServer) Start(ctx context.Context, writeEnabled bool) error {
	s.StartInvoked = true
	return s.StartFn(ctx, writeEnabled)
}
