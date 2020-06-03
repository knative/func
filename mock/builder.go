package mock

type Builder struct {
	BuildInvoked bool
	BuildFn      func(name, path string) (image string, err error)
}

func NewBuilder() *Builder {
	return &Builder{
		BuildFn: func(string, string) (string, error) { return "", nil },
	}
}

func (i *Builder) Build(name, language, path string) (string, error) {
	i.BuildInvoked = true
	return i.BuildFn(name, path)
}
