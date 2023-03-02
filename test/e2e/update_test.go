package e2e

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

const updateTemplatesFolder = "update_templates"

// Update replaces the project content (source files) of the existing project in test
// by the source stored under 'update_template/<runtime>/<template>
// Once sources are update the project is built and re-deployed
func Update(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject) {

	templatePath := filepath.Join(updateTemplatesFolder, project.Runtime, project.Template)
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			// skip update test when there is no template folder
			return
		} else {
			t.Fatal(err.Error())
		}
	}

	// Template folder exists for given runtime / template.
	// Let's update the project and redeploy
	err := UpdateFolderContent(t, templatePath, project)
	if err != nil {
		t.Fatal("an error has occurred while updating project folder with new sources.", err.Error())
	}

	previousRevision := GetCurrentServiceRevision(t, project.FunctionName)

	// Redeploy function
	Deploy(t, knFunc, project)

	// Waits New Revision to become ready
	NewRevisionCheck(t, previousRevision, project.FunctionName)

	// Indicates new project (from update templates) is in use
	project.IsNewRevision = true
}

// UpdateFolderContent overwrites content of project.ProjectPath with content of updatedPath.
// Similar to `cp -a {updatedPath}/* {project.ProjectPath}/`.
func UpdateFolderContent(t *testing.T, updatedPath string, project *FunctionTestProject) error {
	// overwrite files with updated ones
	buff := make([]byte, 4096)
	return filepath.Walk(updatedPath, func(srcPath string, srcFi fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk error: %w", err)
		}
		relPath, err := filepath.Rel(updatedPath, srcPath)
		if err != nil {
			return fmt.Errorf("canot get rel path: %w", err)
		}

		destPath := filepath.Join(project.ProjectPath, relPath)
		destFi, err := os.Lstat(destPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot lstat dest file: %w", err)
		}

		if destFi != nil && destFi.Mode().Type() != srcFi.Mode().Type() {
			return fmt.Errorf("dest file already exist but type is not matching")
		}

		switch srcFi.Mode().Type() {
		case 0:
			var dest, src *os.File
			dest, err = os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, srcFi.Mode().Perm())
			if err != nil {
				return fmt.Errorf("cannot open dest file: %w", err)
			}
			defer dest.Close()
			src, err = os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("cannot open src file: %w", err)
			}
			defer src.Close()
			_, err = io.CopyBuffer(dest, src, buff)
			if err != nil {
				return fmt.Errorf("cannot copy file: %w", err)
			}
		case fs.ModeDir:
			if destFi == nil {
				err = os.MkdirAll(destPath, 0755)
				if err != nil {
					return fmt.Errorf("cannot create dir: %w", err)
				}
			}
		default:
			return fmt.Errorf("unspuported type: %s", srcFi.Mode().Type())
		}
		return nil
	})
}
