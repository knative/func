package mock

import (
	"context"

	fn "knative.dev/func"
)

type Lister struct {
	ListInvoked bool
	ListFn      func() ([]fn.ListItem, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func() ([]fn.ListItem, error) { return []fn.ListItem{}, nil },
	}
}

func (l *Lister) List(context.Context) ([]fn.ListItem, error) {
	l.ListInvoked = true
	return l.ListFn()
}
