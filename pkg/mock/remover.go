package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Remover struct {
	RemoveInvoked bool
	RemoveFn      func(string, string) error
}

func NewRemover() *Remover {
	return &Remover{RemoveFn: func(string, string) error { return nil }}
}

func (r *Remover) Remove(ctx context.Context, name, ns string, _ fn.Function) error {
	r.RemoveInvoked = true
	return r.RemoveFn(name, ns)
}
