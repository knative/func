package functions

import (
	"knative.dev/func/pkg/scaffolding"
)

func LatestMiddlewareVersions() (map[string]map[string]string, error) {
	return scaffolding.MiddlewareVersions(EmbeddedTemplatesFS)
}
