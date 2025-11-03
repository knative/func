package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Lister struct {
	ListInvoked bool
	ListFn      func(context.Context, string) ([]fn.ListItem, bool, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func(context.Context, string) ([]fn.ListItem, bool, error) { return []fn.ListItem{}, true, nil },
	}
}

func (l *Lister) List(ctx context.Context, ns string) ([]fn.ListItem, bool, error) {
	l.ListInvoked = true
	return l.ListFn(ctx, ns)
}
