package functions

// LEGACY PYTHON: detects old parliament functions (func.py with def main(context),
// or a "python -m parliament" Procfile), and rejects the other pre-v1.18
// Procfile-based python layouts (old flask/wsgi templates). Delete this file on
// parliament sunset.

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var (
	// Procfile line launching parliament (python / python3 / python3.x).
	procfileParliamentRe = regexp.MustCompile(`python[0-9.]*\s+-m\s+parliament\b`)
	// parliament import at line start (skips "parliamentarian" and comments).
	pyImportParliamentRe = regexp.MustCompile(`(?m)^[ \t]*(from|import)[ \t]+parliament\b`)
)

// IsLegacyParliament reports whether f.Root holds an old parliament function:
// a "python -m parliament" Procfile, or a top-level *.py importing parliament.
func (f Function) IsLegacyParliament() bool {
	if f.Runtime != "python" {
		return false
	}
	if procfileInvokesParliament(filepath.Join(f.Root, "Procfile")) {
		return true
	}
	return pyImportsParliament(f.Root)
}

// procfileInvokesParliament reports whether the Procfile at path launches parliament.
func procfileInvokesParliament(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return procfileParliamentRe.Match(b)
}

// pyImportsParliament reports whether any top-level *.py at root imports parliament.
func pyImportsParliament(root string) bool {
	matches, err := filepath.Glob(filepath.Join(root, "*.py"))
	if err != nil {
		return false
	}
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if pyImportParliamentRe.Match(b) {
			return true
		}
	}
	return false
}

// ErrUnsupportedLegacyPython rejects pre-v1.18 Procfile-based python layouts other
// than parliament, which would otherwise fail confusingly inside modern scaffolding.
var ErrUnsupportedLegacyPython = errors.New("this function uses a legacy Procfile-based Python layout " +
	"(pre-v1.18, e.g. the old flask or wsgi templates), which is not supported. " +
	"Migrate to the current Python function layout, documented at https://github.com/knative/func/blob/main/docs/function-templates/python.md")

// IsUnsupportedLegacyPython reports whether f.Root holds a pre-v1.18 Procfile-based
// python function other than parliament: a root Procfile with no root pyproject.toml.
// Parliament functions (IsLegacyParliament) are the supported, deprecated exception.
func (f Function) IsUnsupportedLegacyPython() bool {
	if f.Runtime != "python" || f.IsLegacyParliament() {
		return false
	}
	if _, err := os.Stat(filepath.Join(f.Root, "Procfile")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(f.Root, "pyproject.toml"))
	return os.IsNotExist(err)
}
