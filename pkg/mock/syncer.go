package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Syncer struct {
	SyncInvoked bool
	SyncFn      func(fn.Function) error
}

func NewSyncer() *Syncer {
	return &Syncer{SyncFn: func(fn.Function) error { return nil }}
}

func (s *Syncer) Sync(ctx context.Context, f fn.Function) error {
	s.SyncInvoked = true
	return s.SyncFn(f)
}
