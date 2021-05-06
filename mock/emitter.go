package mock

import (
	"context"
)

type Emitter struct {
	EmitInvoked bool
	EmitFn      func(string) error
}

func NewEmitter() *Emitter {
	return &Emitter{
		EmitFn: func(string) error { return nil },
	}
}

func (i *Emitter) Emit(ctx context.Context, s string) error {
	i.EmitInvoked = true
	return i.EmitFn(s)
}
