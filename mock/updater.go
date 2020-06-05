package mock

type Updater struct {
	UpdateInvoked bool
	UpdateFn      func(name, image string) error
}

func NewUpdater() *Updater {
	return &Updater{
		UpdateFn: func(string, string) error { return nil },
	}
}

func (i *Updater) Update(name, image string) error {
	i.UpdateInvoked = true
	return i.UpdateFn(name, image)
}
