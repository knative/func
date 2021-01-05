package mock

import function "github.com/boson-project/func"

type Lister struct {
	ListInvoked bool
	ListFn      func() ([]function.ListItem, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func() ([]function.ListItem, error) { return []function.ListItem{}, nil },
	}
}

func (l *Lister) List() ([]function.ListItem, error) {
	l.ListInvoked = true
	return l.ListFn()
}
