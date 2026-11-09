package mock

import (
	"context"
)

// Process describes a started process. It must stay identical to
// mcp.startedProcess: it is an alias for the same unnamed struct type (rather
// than a named type) so that ProcessStarter satisfies the mcp package's
// process-starter interface without this package importing it.
type Process = struct {
	Pid        int
	Host, Port string
	Stop       func() error
	Exited     <-chan struct{}
	Output     func() (lines string, truncated bool)
}

// ProcessStarter is a mock implementation of the process-starter interface
// used by the "run" tool. It avoids spawning real subprocesses in tests.
// It implements the same interface as mcp.processStarter through structural
// typing.
type ProcessStarter struct {
	StartInvoked bool
	StartFn      func(ctx context.Context, subcommand string, args ...string) (Process, error)
}

// NewProcessStarter creates a new mock process starter.
func NewProcessStarter() *ProcessStarter {
	return &ProcessStarter{}
}

// Start implements the processStarter interface, recording invocation
// details and delegating to StartFn if provided.
func (m *ProcessStarter) Start(ctx context.Context, subcommand string, args ...string) (Process, error) {
	m.StartInvoked = true

	if m.StartFn != nil {
		return m.StartFn(ctx, subcommand, args...)
	}

	return Process{Pid: 1234, Host: "127.0.0.1", Port: "8080", Stop: func() error { return nil }}, nil
}
