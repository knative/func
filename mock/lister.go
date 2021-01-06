package mock

import bosonFunc "github.com/boson-project/func"

type Lister struct {
	ListInvoked bool
	ListFn      func() ([]bosonFunc.ListItem, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func() ([]bosonFunc.ListItem, error) { return []bosonFunc.ListItem{}, nil },
	}
}

func (l *Lister) List() ([]bosonFunc.ListItem, error) {
	l.ListInvoked = true
	return l.ListFn()
}
