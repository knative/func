package mock

import (
	"context"
)

// ProcessStarter is a mock implementation of the process-starter interface
// used by the "run" tool. It avoids spawning real subprocesses in tests.
// It implements the same interface as mcp.processStarter through structural
// typing.
type ProcessStarter struct {
	StartInvoked bool
	StartFn      func(ctx context.Context, subcommand string, args ...string) (pid int, host, port string, stop func() error, err error)
}

// NewProcessStarter creates a new mock process starter.
func NewProcessStarter() *ProcessStarter {
	return &ProcessStarter{}
}

// Start implements the processStarter interface, recording invocation
// details and delegating to StartFn if provided.
func (m *ProcessStarter) Start(ctx context.Context, subcommand string, args ...string) (pid int, host, port string, stop func() error, err error) {
	m.StartInvoked = true

	if m.StartFn != nil {
		return m.StartFn(ctx, subcommand, args...)
	}

	return 1234, "127.0.0.1", "8080", func() error { return nil }, nil
}
