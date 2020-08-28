package mock

import "github.com/boson-project/faas"

type Updater struct {
	UpdateInvoked bool
	UpdateFn      func(faas.Function) error
}

func NewUpdater() *Updater {
	return &Updater{
		UpdateFn: func(faas.Function) error { return nil },
	}
}

func (i *Updater) Update(f faas.Function) error {
	i.UpdateInvoked = true
	return i.UpdateFn(f)
}
