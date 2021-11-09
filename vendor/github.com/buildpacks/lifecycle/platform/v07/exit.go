package v07

import (
	"github.com/buildpacks/lifecycle/cmd"
)

func (p *v07Platform) CodeFor(errType cmd.LifecycleExitError) int {
	return p.previousPlatform.CodeFor(errType)
}
