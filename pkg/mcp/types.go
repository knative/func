package mcp

import fn "knative.dev/func/pkg/functions"

// ClientConfig carries settings used when constructing a functions client.
type ClientConfig struct {
	Verbose            bool
	InsecureSkipVerify bool
}

// ClientFactory constructs a functions client with optional overrides.
// The returned cleanup function must be called when the client is no longer needed.
type ClientFactory func(cfg ClientConfig, options ...fn.Option) (*fn.Client, func())
