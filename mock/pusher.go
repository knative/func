package mock

import "github.com/boson-project/faas"

type Pusher struct {
	PushInvoked bool
	PushFn      func(faas.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(faas.Function) (string, error) { return "", nil },
	}
}

func (i *Pusher) Push(f faas.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(f)
}
