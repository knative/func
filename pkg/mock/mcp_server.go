package mock

import (
	"context"
)

type MCPServer struct {
	StartInvoked bool
	StartFn      func(context.Context) error
}

func NewMCPServer() *MCPServer {
	return &MCPServer{
		StartFn: func(context.Context) error { return nil },
	}
}

func (s *MCPServer) Start(ctx context.Context) error {
	s.StartInvoked = true
	return s.StartFn(ctx)
}
