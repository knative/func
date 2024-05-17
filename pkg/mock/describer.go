package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Describer struct {
	DescribeInvoked bool
	DescribeFn      func(context.Context, string, string) (fn.Instance, error)
}

func NewDescriber() *Describer {
	return &Describer{
		DescribeFn: func(context.Context, string, string) (fn.Instance, error) { return fn.Instance{}, nil },
	}
}

func (l *Describer) Describe(ctx context.Context, name, namespace string) (fn.Instance, error) {
	l.DescribeInvoked = true
	return l.DescribeFn(ctx, name, namespace)
}
