package extend

import "time"

type Options struct {
	IgnorePaths []string
	CacheTTL    time.Duration
}
