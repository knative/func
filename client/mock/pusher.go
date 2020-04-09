package mock

type Pusher struct {
	PushInvoked bool
	PushFn      func(image string) error
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(string) error { return nil },
	}
}

func (i *Pusher) Push(image string) error {
	i.PushInvoked = true
	return i.PushFn(image)
}
