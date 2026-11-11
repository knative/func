package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Syncer struct {
	SyncInvoked   bool
	SyncFn        func(fn.Function) error
	UnsyncInvoked bool
	UnsyncFn      func(name, namespace string) error
}

func NewSyncer() *Syncer {
	return &Syncer{
		SyncFn:   func(fn.Function) error { return nil },
		UnsyncFn: func(string, string) error { return nil },
	}
}

func (s *Syncer) Sync(ctx context.Context, f fn.Function) error {
	s.SyncInvoked = true
	return s.SyncFn(f)
}

func (s *Syncer) Unsync(ctx context.Context, name, namespace string) error {
	s.UnsyncInvoked = true
	return s.UnsyncFn(name, namespace)
}
