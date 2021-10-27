package v07

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type v07Platform struct {
	api              *api.Version
	previousPlatform common.Platform
}

func NewPlatform(previousPlatform common.Platform) common.Platform {
	return &v07Platform{
		api:              api.MustParse("0.7"),
		previousPlatform: previousPlatform,
	}
}

func (p *v07Platform) API() string {
	return p.api.String()
}
