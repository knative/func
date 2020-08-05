package mock

type Pusher struct {
	PushInvoked bool
	PushFn      func(tag string) error
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(tag string) error { return nil },
	}
}

func (i *Pusher) Push(tag string) error {
	i.PushInvoked = true
	return i.PushFn(tag)
}
