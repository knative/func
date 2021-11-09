package common

import (
	"github.com/buildpacks/lifecycle/cmd"
)

type Platform interface {
	API() string
	CodeFor(errType cmd.LifecycleExitError) int
}
