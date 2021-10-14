package mock

import (
	"context"
)

type Emitter struct {
	SendInvoked bool
	SendFn      func(string) error
}

func NewEmitter() *Emitter {
	return &Emitter{
		SendFn: func(string) error { return nil },
	}
}

func (i *Emitter) Send(ctx context.Context, s string) error {
	i.SendInvoked = true
	return i.SendFn(s)
}
