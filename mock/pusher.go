package mock

import bosonFunc "github.com/boson-project/func"

type Pusher struct {
	PushInvoked bool
	PushFn      func(bosonFunc.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(bosonFunc.Function) (string, error) { return "", nil },
	}
}

func (i *Pusher) Push(f bosonFunc.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(f)
}
