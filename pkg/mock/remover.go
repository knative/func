package mock

import "context"

type Remover struct {
	RemoveInvoked bool
	RemoveFn      func(string, string) (bool, error)
}

func NewRemover() *Remover {
	return &Remover{RemoveFn: func(string, string) (bool, error) { return true, nil }}
}

func (r *Remover) Remove(ctx context.Context, name, ns string) (bool, error) {
	r.RemoveInvoked = true
	return r.RemoveFn(name, ns)
}
