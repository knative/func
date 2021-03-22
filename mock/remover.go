package mock

import "context"

type Remover struct {
	RemoveInvoked bool
	RemoveFn      func(string) error
}

func NewRemover() *Remover {
	return &Remover{}
}

func (r *Remover) Remove(ctx context.Context, name string) error {
	r.RemoveInvoked = true
	return r.RemoveFn(name)
}
