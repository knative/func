package mock

import (
	"context"
)

// Executor is a mock implementation of the command executor interface.
// It implements the same interface as mcp.Executor through structural typing.
type Executor struct {
	ExecuteInvoked bool
	ExecuteFn      func(context.Context, string, ...string) ([]byte, error)

	ExecuteSplitInvoked bool
	ExecuteSplitFn      func(context.Context, string, ...string) (stdout, stderr []byte, err error)

	ExecuteEnvInvoked bool
	ExecuteEnvFn      func(ctx context.Context, env []string, subcommand string, args ...string) ([]byte, error)
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

// ExecuteEnv implements the executor interface, recording invocation details
// and delegating to ExecuteEnvFn if provided. Without ExecuteEnvFn it falls
// back to Execute, so tests which only check arguments need not change.
func (m *Executor) ExecuteEnv(ctx context.Context, env []string, subcommand string, args ...string) ([]byte, error) {
	m.ExecuteEnvInvoked = true

	if m.ExecuteEnvFn != nil {
		return m.ExecuteEnvFn(ctx, env, subcommand, args...)
	}

	return m.Execute(ctx, subcommand, args...)
}

// ExecuteSplit implements the executor interface, recording invocation
// details and delegating to ExecuteSplitFn if provided.
func (m *Executor) ExecuteSplit(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
	m.ExecuteSplitInvoked = true

	if m.ExecuteSplitFn != nil {
		return m.ExecuteSplitFn(ctx, subcommand, args...)
	}

	return []byte(""), []byte(""), nil
}
