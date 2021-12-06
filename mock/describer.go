package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Describer struct {
	DescribeInvoked bool
	DescribeFn      func(string) (fn.Info, error)
}

func NewDescriber() *Describer {
	return &Describer{
		DescribeFn: func(string) (fn.Info, error) { return fn.Info{}, nil },
	}
}

func (l *Describer) Describe(_ context.Context, name string) (fn.Info, error) {
	l.DescribeInvoked = true
	return l.DescribeFn(name)
}
