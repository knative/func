package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type PipelinesProvider struct {
	RunInvoked          bool
	RunFn               func(fn.Function) error
	RemoveInvoked       bool
	RemoveFn            func(fn.Function) error
	ConfigurePACInvoked bool
	ConfigurePACFn      func(fn.Function) error
}

func NewPipelinesProvider() *PipelinesProvider {
	return &PipelinesProvider{
		RunFn:          func(fn.Function) error { return nil },
		RemoveFn:       func(fn.Function) error { return nil },
		ConfigurePACFn: func(fn.Function) error { return nil },
	}
}

func (p *PipelinesProvider) Run(ctx context.Context, f fn.Function) error {
	p.RunInvoked = true
	return p.RunFn(f)
}

func (p *PipelinesProvider) Remove(ctx context.Context, f fn.Function) error {
	p.RemoveInvoked = true
	return p.RemoveFn(f)
}
func (p *PipelinesProvider) ConfigurePAC(ctx context.Context, f fn.Function, metadata any) error {
	p.ConfigurePACInvoked = true
	return p.ConfigurePACFn(f)
}
