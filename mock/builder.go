package mock

import function "github.com/boson-project/func"

type Builder struct {
	BuildInvoked bool
	BuildFn      func(function.Function) error
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(function.Function) error { return nil },
	}
}

func (i *Builder) Build(f function.Function) error {
	i.BuildInvoked = true
	return i.BuildFn(f)
}
