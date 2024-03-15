package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Lister struct {
	ListInvoked bool
	ListFn      func(context.Context, string) ([]fn.ListItem, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func(context.Context, string) ([]fn.ListItem, error) { return []fn.ListItem{}, nil },
	}
}

func (l *Lister) List(ctx context.Context, ns string) ([]fn.ListItem, error) {
	l.ListInvoked = true
	return l.ListFn(ctx, ns)
}
