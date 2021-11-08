package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Client struct {
	// Members used to confirm certain configuration was used for instantiation
	// (roughly map to the real clients WithX functions)
	Confirm          bool
	RepositoriesPath string

	// repositories manager accessor
	repositories *Repositories
}

func NewClient() *Client {
	return &Client{repositories: NewRepositories()}
}

func (c *Client) Repositories() *Repositories {
	return c.repositories
}

type Repositories struct {
	// Members which record whether or not the various methods were invoked.
	ListInvoked bool

	all []fn.IRepository
}

func NewRepositories() *Repositories {
	return &Repositories{all: []fn.IRepository{fn.NewRepo("default")}}
}

func (r *Repositories) All() ([]fn.IRepository, error) {
	return r.all, nil
}

func (r *Repositories) List() ([]string, error) {
	r.ListInvoked = true
	names := []string{}
	for _, v := range r.all {
		names = append(names, v.Name())
	}
	return names, nil
}

func (r *Repositories) Add(name, url string) (string, error) {
	r.all = append(r.all, fn.NewRepo(name))
	return "", nil
}

func (r *Repositories) Rename(old, new string) error {
	for i, v := range r.all {
		if v.Name() == old {
			r.all[i] = renamedRepo{impl: v, newName: new}
		}
	}
	return nil
}

type renamedRepo struct {
	impl    fn.IRepository
	newName string
}

func (r renamedRepo) Name() string {
	return r.newName
}

func (r renamedRepo) URL() string {
	return r.impl.URL()
}

func (r renamedRepo) Runtimes(ctx context.Context) ([]string, error) {
	return r.impl.Runtimes(ctx)
}

func (r renamedRepo) Templates(ctx context.Context, runtime string) ([]string, error) {
	return r.impl.Templates(ctx, runtime)
}

func (r renamedRepo) Template(ctx context.Context, runtime, name string) (fn.ITemplate, error) {
	return r.impl.Template(ctx, runtime, name)
}

func (r *Repositories) Remove(name string) error {
	repos := []fn.IRepository{}
	for _, v := range r.all {
		if v.Name() == name {
			continue
		}
		repos = append(repos, v)
	}
	r.all = repos
	return nil
}
