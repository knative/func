package platform

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	v05 "github.com/buildpacks/lifecycle/platform/v05"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
)

type Platform interface {
	API() string
	CodeFor(errType cmd.LifecycleExitError) int
}

func NewPlatform(apiStr string) Platform {
	platformAPI := api.MustParse(apiStr)
	if platformAPI.Compare(api.MustParse("0.6")) < 0 { // platform API < 0.6
		return v05.NewPlatform(apiStr)
	}
	return v06.NewPlatform(apiStr)
}
