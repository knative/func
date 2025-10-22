package mock

import (
	"context"
)

// Executor is a mock implementation of the command executor interface.
// It implements the same interface as mcp.Executor through structural typing.
type Executor struct {
	ExecuteInvoked bool
	ExecuteFn      func(context.Context, string, ...string) ([]byte, error)
}

// NewExecutor creates a new mock executor
func NewExecutor() *Executor {
	return &Executor{}
}

// Execute implements the executor interface, recording invocation details
// and delegating to ExecuteFn if provided.
func (m *Executor) Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	m.ExecuteInvoked = true

	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, subcommand, args...)
	}

	return []byte(""), nil
}
