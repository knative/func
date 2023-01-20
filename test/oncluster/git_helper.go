package oncluster

import (
	"testing"

	fn "knative.dev/func"
	common "knative.dev/func/test/common"
)

// UpdateFuncGit updates a function's git settings
func UpdateFuncGit(t *testing.T, projectDir string, git fn.Git) {
	f, err := fn.NewFunction(projectDir)
	AssertNoError(t, err)
	f.Build.Git = git
	err = f.Write()
	AssertNoError(t, err)
}

// GitInitialCommitAndPush Runs repeatable git commands used on every initial repository setup
// such as `git init`, `git config user`, `git add .`, `git remote add ...` and `git push`
func GitInitialCommitAndPush(t *testing.T, gitProjectPath string, originCloneURL string) (sh *common.TestExecCmd) {

	sh = common.NewShellCmd(t, gitProjectPath)
	sh.ShouldFailOnError = true
	sh.ShouldDumpOnSuccess = false
	sh.Exec(`git init`)
	sh.Exec(`git branch -M main`)
	sh.Exec(`git add .`)
	sh.Exec(`git config user.name "John Smith"`)
	sh.Exec(`git config user.email "john.smith@example.com"`)
	sh.Exec(`git commit -m "initial commit"`)
	sh.Exec(`git remote add origin ` + originCloneURL)
	sh.Exec(`git push -u origin main`)
	return sh

}
