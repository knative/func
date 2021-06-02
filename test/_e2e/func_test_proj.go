package e2e

import (
	"fmt"
	"os"
	"path/filepath"
)

// FunctionTestProject
// structure to represent a function testing project location
// stored on filesystem
type FunctionTestProject struct {

	// Function Name. Example "func-node-http"
	FunctionName string
	// Project location on filesystem.
	// Example /tmp/func-node-http | %USERPROFILE%\AddData\Local\Temp\func-node-http
	ProjectPath string
	// Function Runtime. Example "node"
	Runtime string
	// Function Trigger. Example "http"
	Trigger string
	// Indicates function is already deployed
	IsDeployed bool
	// Indicates new revision deployed (custom template)
	IsNewRevision bool
	// Function URL
	FunctionURL string

}

// NewFunctionTestProject initiates a project with derived function name an project path
func NewFunctionTestProject(runtime string, trigger string) FunctionTestProject {
	project := FunctionTestProject{
		Runtime: runtime,
		Trigger: trigger,
	}
	project.FunctionName = "func-" + runtime + "-" + trigger
	project.ProjectPath = filepath.Join(os.TempDir(), project.FunctionName)
	return project
}

// ExistsProjectFolder determine the project folder exists or not
func (f FunctionTestProject) ExistsProjectFolder() bool {
	fileInfo, _ := os.Stat(f.ProjectPath)
	if fileInfo != nil && fileInfo.IsDir() {
		return true
	}
	return false
}

// CreateProjectFolder creates and empty folder for the project location.
func (f FunctionTestProject) CreateProjectFolder() error {
	if f.ProjectPath != "" {
		return os.Mkdir(f.ProjectPath, 0755)
	}
	return nil
}

// RemoveProjectFolder removes existing project folder
func (f FunctionTestProject) RemoveProjectFolder() error {
	if f.ProjectPath != "" {
		err := os.RemoveAll(f.ProjectPath)
		if err != nil  && !os.IsNotExist(err) {
			return fmt.Errorf("unable to remove project folder: %s", err.Error())
		}
	}
	return nil
}
