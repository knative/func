package mock

import bosonFunc "github.com/boson-project/func"

type Builder struct {
	BuildInvoked bool
	BuildFn      func(bosonFunc.Function) error
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(bosonFunc.Function) error { return nil },
	}
}

func (i *Builder) Build(f bosonFunc.Function) error {
	i.BuildInvoked = true
	return i.BuildFn(f)
}
