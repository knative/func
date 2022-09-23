package platform

import (
	"github.com/buildpacks/lifecycle/api"
)

// Platform handles logic pertaining to inputs and outputs from a platform (lifecycle invoker)'s perspective.
type Platform struct {
	*InputsResolver
	Exiter
	api *api.Version
}

// NewPlatform accepts a platform API and returns a new Platform.
func NewPlatform(apiStr string) *Platform {
	platformAPI := api.MustParse(apiStr)
	return &Platform{
		InputsResolver: NewInputsResolver(platformAPI),
		Exiter:         NewExiter(apiStr),
		api:            platformAPI,
	}
}

// API returns the platform API.
func (p *Platform) API() *api.Version {
	return p.api
}

// InputsResolver resolves inputs for each of the lifecycle phases.
type InputsResolver struct {
	platformAPI *api.Version
}

// NewInputsResolver accepts a platform API and returns a new InputsResolver.
func NewInputsResolver(platformAPI *api.Version) *InputsResolver {
	return &InputsResolver{platformAPI: platformAPI}
}
