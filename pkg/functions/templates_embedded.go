package functions

import (
	"knative.dev/func/pkg/filesystem"
	functemplates "knative.dev/func/templates"
)

var EmbeddedTemplatesFS filesystem.Filesystem = filesystem.NewManglingFS(functemplates.Content)
