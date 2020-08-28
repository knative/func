package mock

import "github.com/boson-project/faas"

type Builder struct {
	BuildInvoked bool
	BuildFn      func(faas.Function) error
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(faas.Function) error { return nil },
	}
}

func (i *Builder) Build(f faas.Function) error {
	i.BuildInvoked = true
	return i.BuildFn(f)
}
