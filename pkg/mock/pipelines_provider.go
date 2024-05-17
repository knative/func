package mock

import (
	"context"
	"errors"

	fn "knative.dev/func/pkg/functions"
)

type PipelinesProvider struct {
	RunInvoked          bool
	RunFn               func(fn.Function) (string, fn.Function, error)
	RemoveInvoked       bool
	RemoveFn            func(fn.Function) error
	ConfigurePACInvoked bool
	ConfigurePACFn      func(fn.Function) error
	RemovePACInvoked    bool
	RemovePACFn         func(fn.Function) error
}

func NewPipelinesProvider() *PipelinesProvider {
	return &PipelinesProvider{
		RunFn: func(f fn.Function) (string, fn.Function, error) {
			// the minimum necessary logic for a deployer, which should be
			// confirmed by tests in the respective implementations, is to
			// return the function with f.Deploy.* values set reflecting the
			// now deployed state of the function.
			if f.Namespace == "" && f.Deploy.Namespace == "" {
				return "", f, errors.New("namespace required for initial deployment")
			}

			// fabricate that we deployed it to the newly requested namespace
			if f.Namespace != "" {
				f.Deploy.Namespace = f.Namespace
			}

			// fabricate that we deployed the requested image or generated
			// it as needed
			var err error
			if f.Image != "" {
				f.Deploy.Image = f.Image
			} else {
				if f.Deploy.Image, err = f.ImageName(); err != nil {
					return "", f, err
				}
			}

			return "", f, nil

		},
		RemoveFn:       func(fn.Function) error { return nil },
		ConfigurePACFn: func(fn.Function) error { return nil },
		RemovePACFn:    func(fn.Function) error { return nil },
	}
}

func (p *PipelinesProvider) Run(ctx context.Context, f fn.Function) (string, fn.Function, error) {
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

func (p *PipelinesProvider) RemovePAC(ctx context.Context, f fn.Function, metadata any) error {
	p.RemovePACInvoked = true
	return p.RemovePACFn(f)
}
