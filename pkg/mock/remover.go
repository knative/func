package mock

import "context"

type Remover struct {
	RemoveInvoked bool
	RemoveFn      func(string, string) error
}

func NewRemover() *Remover {
	return &Remover{RemoveFn: func(string, string) error { return nil }}
}

func (r *Remover) Remove(ctx context.Context, name, ns string) error {
	r.RemoveInvoked = true
	return r.RemoveFn(name, ns)
}
