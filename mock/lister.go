package mock

import "github.com/boson-project/faas"

type Lister struct {
	ListInvoked bool
	ListFn      func() ([]faas.ListItem, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func() ([]faas.ListItem, error) { return []faas.ListItem{}, nil },
	}
}

func (l *Lister) List() ([]faas.ListItem, error) {
	l.ListInvoked = true
	return l.ListFn()
}
