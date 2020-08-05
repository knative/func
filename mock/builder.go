package mock

type Builder struct {
	BuildInvoked bool
	BuildFn      func(tag string) (image string, err error)
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(string) (string, error) { return "", nil },
	}
}

func (i *Builder) Build(tag string) (string, error) {
	i.BuildInvoked = true
	return i.BuildFn(tag)
}
