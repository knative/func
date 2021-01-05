package mock

import function "github.com/boson-project/func"

type Pusher struct {
	PushInvoked bool
	PushFn      func(function.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(function.Function) (string, error) { return "", nil },
	}
}

func (i *Pusher) Push(f function.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(f)
}
