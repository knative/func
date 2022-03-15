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
	ExportInvoked bool
	ExportFn      func(fn.Function) error
}

func NewPipelinesProvider() *PipelinesProvider {
	return &PipelinesProvider{
		RunFn:    func(fn.Function) error { return nil },
		RemoveFn: func(fn.Function) error { return nil },
		ExportFn: func(f fn.Function) error { return nil },
	}
}

func (p *PipelinesProvider) Run(ctx context.Context, f fn.Function, b bool) error {
	p.RunInvoked = true
	return p.RunFn(f)
}

func (p *PipelinesProvider) Remove(ctx context.Context, f fn.Function) error {
	p.RemoveInvoked = true
	return p.RemoveFn(f)
}

func (p *PipelinesProvider) Export(ctx context.Context, f fn.Function, namespace string) error {
	p.ExportInvoked = true
	return p.ExportFn(f)
}
