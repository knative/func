package mock

import (
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

	all []fn.Repository
}

func NewRepositories() *Repositories {
	return &Repositories{all: []fn.Repository{{Name: "default"}}}
}

func (r *Repositories) All() ([]fn.Repository, error) {
	return r.all, nil
}

func (r *Repositories) List() ([]string, error) {
	r.ListInvoked = true
	names := []string{}
	for _, v := range r.all {
		names = append(names, v.Name)
	}
	return names, nil
}

func (r *Repositories) Add(name, url string) error {
	r.all = append(r.all, fn.Repository{Name: name, URL: url})
	return nil
}

func (r *Repositories) Rename(old, new string) error {
	for i, v := range r.all {
		if v.Name == old {
			v.Name = new
			r.all[i] = v
		}
	}
	return nil
}

func (r *Repositories) Remove(name string) error {
	repos := []fn.Repository{}
	for _, v := range r.all {
		if v.Name == name {
			continue
		}
		repos = append(repos, v)
	}
	r.all = repos
	return nil
}
