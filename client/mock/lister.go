package mock

type Lister struct {
	ListInvoked bool
	ListFn      func() ([]string, error)
}

func NewLister() *Lister {
	return &Lister{
		ListFn: func() ([]string, error) { return []string{}, nil },
	}
}

func (l *Lister) List() ([]string, error) {
	l.ListInvoked = true
	return l.ListFn()
}
