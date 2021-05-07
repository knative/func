package mock

import (
	"context"
	fn "github.com/boson-project/func"
)

type Builder struct {
	BuildInvoked bool
	BuildFn      func(fn.Function) error
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(fn.Function) error { return nil },
	}
}

func (i *Builder) Build(ctx context.Context, f fn.Function) error {
	i.BuildInvoked = true
	return i.BuildFn(f)
}
