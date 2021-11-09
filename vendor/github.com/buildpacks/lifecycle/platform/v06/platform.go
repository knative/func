package v06

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type v06Platform struct {
	api              *api.Version
	previousPlatform common.Platform
}

func NewPlatform(previousPlatform common.Platform) common.Platform {
	return &v06Platform{
		api:              api.MustParse("0.6"),
		previousPlatform: previousPlatform,
	}
}

func (p *v06Platform) API() string {
	return p.api.String()
}
