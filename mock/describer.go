package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Describer struct {
	DescribeInvoked bool
	DescribeFn      func(string) (fn.Instance, error)
}

func NewDescriber() *Describer {
	return &Describer{
		DescribeFn: func(string) (fn.Instance, error) { return fn.Instance{}, nil },
	}
}

func (l *Describer) Describe(_ context.Context, name string) (fn.Instance, error) {
	l.DescribeInvoked = true
	return l.DescribeFn(name)
}
