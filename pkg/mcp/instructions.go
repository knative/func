package mcp

import _ "embed"

//go:embed instructions_warning.md
var readonlyWarning string

//go:embed instructions.md
var instructionsBody string

func instructions(readonly bool) string {
	if readonly {
		return readonlyWarning + instructionsBody
	}
	return instructionsBody
}
