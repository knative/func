package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Pusher struct {
	PushInvoked bool
	PushFn      func(context.Context, fn.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(context.Context, fn.Function) (string, error) { return "", nil },
	}
}

func (i *Pusher) Push(ctx context.Context, f fn.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(ctx, f)
}
