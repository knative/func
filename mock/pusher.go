package mock

import (
	"context"

	fn "github.com/boson-project/func"
)

type Pusher struct {
	PushInvoked bool
	PushFn      func(fn.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(fn.Function) (string, error) { return "", nil },
	}
}

func (i *Pusher) Push(ctx context.Context, f fn.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(f)
}
