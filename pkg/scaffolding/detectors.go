package scaffolding

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// detector of method signatures.  Each instance is for a given runtime.
type detector interface {
	Detect(dir string) (static, instanced bool, err error)
}

// newDetector returns a deector instance for the given runtime.
func newDetector(runtime string) (detector, error) {
	switch runtime {
	case "go":
		return &goDetector{}, nil
	case "python":
		return &pythonDetector{}, nil
	case "rust":
		return nil, ErrDetectorNotImplemented{runtime}
	case "node":
		return nil, ErrDetectorNotImplemented{runtime}
	case "typescript":
		return nil, ErrDetectorNotImplemented{runtime}
	case "quarkus":
		return nil, ErrDetectorNotImplemented{runtime}
	case "java":
		return nil, ErrDetectorNotImplemented{runtime}
	default:
		return nil, ErrRuntimeNotRecognized{runtime}
	}
}

// GO

type goDetector struct{}

func (d goDetector) Detect(dir string) (static, instanced bool, err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return static, instanced, fmt.Errorf("signature detector encountered an error when scanning the function's source code. %w", err)
	}
	for _, file := range files {
		filename := filepath.Join(dir, file.Name())
		if file.IsDir() || !strings.HasSuffix(filename, ".go") {
			continue
		}
		if d.hasFunctionDeclaration(filename, "New") {
			instanced = true
		}
		if d.hasFunctionDeclaration(filename, "Handle") {
			static = true
		}
	}
	return
}

func (d goDetector) hasFunctionDeclaration(filename, function string) bool {
	astFile, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, decl := range astFile.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Name matches and it has no reciver.  I.e. a package level function
			if funcDecl.Name.Name == function && funcDecl.Recv == nil {
				return true
			}
		}
	}
	return false
}

// PYTHON

type pythonDetector struct{}

func (d pythonDetector) Detect(dir string) (bool, bool, error) {
	return false, false, errors.New("the Python method signature detector is not yet available")
}

// TODO: Other Detectors
