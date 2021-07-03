package mock

import (
	"context"
	fn "github.com/boson-project/func"
)

// Client mocks the whole client
// (see cli tests)
type Client struct {
	NewFn      func(context.Context, fn.Function) error
	CreateFn   func(fn.Function) error
	BuildFn    func(context.Context, fn.Function) error
	DeployFn   func(context.Context, string) error
	RunFn      func(context.Context, string) error
	ListFn     func(context.Context) ([]fn.ListItem, error)
	DescribeFn func(context.Context, string) (fn.Description, error)
	RemoveFn   func(context.Context, fn.Function) error
}

func NewClient() *Client {
	return &Client{
		NewFn:    func(context.Context, fn.Function) error { return nil },
		CreateFn: func(fn.Function) error { return nil },
		BuildFn:  func(context.Context, fn.Function) error { return nil },
		DeployFn: func(context.Context, string) error { return nil },
		RunFn:    func(context.Context, string) error { return nil },
		ListFn:   func(context.Context) ([]fn.ListItem, error) { return []fn.ListItem{}, nil },
		DescribeFn: func(context.Context, string) (fn.Description, error) {
			return fn.Description{}, nil
		},
		RemoveFn: func(context.Context, fn.Function) error { return nil },
	}
}

func (c *Client) New(ctx context.Context, f fn.Function) error {
	return c.NewFn(ctx, f)
}

func (c *Client) Create(f fn.Function) error {
	return c.CreateFn(f)
}

func (c *Client) Build(ctx context.Context, f fn.Function) error {
	return c.BuildFn(ctx, f)
}

func (c *Client) Deploy(ctx context.Context, path string) error {
	return c.DeployFn(ctx, path)
}

func (c *Client) Run(ctx context.Context, root string) error {
	return c.RunFn(ctx, root)
}

func (c *Client) List(ctx context.Context) ([]fn.ListItem, error) {
	return c.ListFn(ctx)
}

func (c *Client) Describe(ctx context.Context, name string) (fn.Description, error) {
	return c.DescribeFn(ctx, name)
}

func (c *Client) Remove(ctx context.Context, f fn.Function) error {
	return c.RemoveFn(ctx, f)
}
