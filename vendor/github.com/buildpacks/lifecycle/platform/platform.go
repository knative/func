package platform

import (
	"fmt"

	"github.com/buildpacks/lifecycle/platform/common"
	"github.com/buildpacks/lifecycle/platform/pre06"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
	v07 "github.com/buildpacks/lifecycle/platform/v07"
)

var platform03 = pre06.NewPlatform("0.3")
var platform04 = pre06.NewPlatform("0.4")
var platform05 = pre06.NewPlatform("0.5")
var platform06 = v06.NewPlatform(platform05)
var platform07 = v07.NewPlatform(platform06)

var supportedPlatforms = map[string]common.Platform{
	"0.3": platform03,
	"0.4": platform04,
	"0.5": platform05,
	"0.6": platform06,
	"0.7": platform07,
}

func NewPlatform(apiStr string) (common.Platform, error) {
	p, ok := supportedPlatforms[apiStr]
	if !ok {
		return nil, fmt.Errorf("unable to create platform for api %s: unknown api", apiStr)
	}
	return p, nil
}
