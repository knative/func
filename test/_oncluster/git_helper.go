package oncluster

import (
	"fmt"
	"os"
	"testing"

	yaml "gopkg.in/yaml.v2"
	common "knative.dev/kn-plugin-func/test/_common"
)

type Git struct {
	URL        string
	Revision   string
	ContextDir string
}

// UpdateFuncYamlGit update func.yaml file by setting build to git as well as git fields.
func UpdateFuncYamlGit(t *testing.T, projectDir string, git Git) {

	funcYamlPath := projectDir + "/func.yaml"
	data, err := os.ReadFile(funcYamlPath)
	AssertNoError(t, err)

	m := make(map[interface{}]interface{})
	err = yaml.Unmarshal([]byte(data), &m)
	AssertNoError(t, err)

	gitMap := make(map[interface{}]interface{})
	m["build"] = "git"
	m["git"] = gitMap

	changeLog := fmt.Sprintln("build:", "git")
	updateGitField := func(targetField string, targetValue string) {
		if targetValue != "" {
			gitMap[targetField] = targetValue
			changeLog += fmt.Sprintln("git.", targetField, ":", targetValue)
		}
	}
	updateGitField("url", git.URL)
	updateGitField("revision", git.Revision)
	updateGitField("contextDir", git.ContextDir)

	outData, _ := yaml.Marshal(m)
	err = os.WriteFile(funcYamlPath, outData, 0644)
	AssertNoError(t, err)
	t.Logf("func.yaml changed:\n%v", changeLog)
}

// GitInitialCommitAndPush Runs repeatable git commands used on every initial repository setup
// such as `git init`, `git config user`, `git add .`, `git remote add ...` and `git push`
func GitInitialCommitAndPush(t *testing.T, gitProjectPath string, originCloneURL string) (sh *common.TestExecCmd) {

	sh = common.NewShellCmd(t, gitProjectPath)
	sh.ShouldFailOnError = true
	sh.ShouldDumpOnSuccess = true
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
