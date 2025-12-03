package ci

import (
	"path/filepath"

	"github.com/ory/viper"
)

const (
	ConfigCIFeatureFlag = "FUNC_ENABLE_CI_CONFIG"

	DefaultGithubWorkflowDir      = ".github/workflows"
	DefaultGithubWorkflowFilename = "func-deploy.yaml"

	PathOption = "path"

	WorkflowNameOption  = "workflow-name"
	DefaultWorkflowName = "Func Deploy"
)

// CIConfig readonly CI configuration
type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFilename,
	path,
	workflowName string
}

func NewCiGithubConfig() CIConfig {
	return CIConfig{
		githubWorkflowDir:      DefaultGithubWorkflowDir,
		githubWorkflowFilename: DefaultGithubWorkflowFilename,
		path:                   viper.GetString(PathOption),
		workflowName:           viper.GetString(WorkflowNameOption),
	}
}

func (cc *CIConfig) FnGithubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGithubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGithubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
}

func (cc *CIConfig) Path() string {
	return cc.path
}
