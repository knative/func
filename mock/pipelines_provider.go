package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type PipelinesProvider struct {
	RunInvoked    bool
	RunFn         func(fn.Function) error
	RemoveInvoked bool
	RemoveFn      func(fn.Function) error
}

func NewPipelinesProvider() *PipelinesProvider {
	return &PipelinesProvider{
		RunFn:    func(fn.Function) error { return nil },
		RemoveFn: func(fn.Function) error { return nil },
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
