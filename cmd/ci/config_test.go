package ci

import (
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// ---------------------------------------------------------------------------
// Pure accessor methods on a hand-built CIConfig
// ---------------------------------------------------------------------------

func TestCIConfig_Verbose(t *testing.T) {
	assert.Assert(t, !CIConfig{verbose: false}.Verbose())
	assert.Assert(t, CIConfig{verbose: true}.Verbose())
}

func TestCIConfig_FnRuntime(t *testing.T) {
	assert.Equal(t, CIConfig{fnRuntime: "go"}.FnRuntime(), "go")
	assert.Equal(t, CIConfig{fnRuntime: "python"}.FnRuntime(), "python")
}

func TestCIConfig_FnBuilder(t *testing.T) {
	assert.Equal(t, CIConfig{fnBuilder: "host"}.FnBuilder(), "host")
	assert.Equal(t, CIConfig{fnBuilder: "pack"}.FnBuilder(), "pack")
	assert.Equal(t, CIConfig{fnBuilder: "s2i"}.FnBuilder(), "s2i")
}

func TestCIConfig_OutputPath(t *testing.T) {
	cc := CIConfig{
		githubWorkflowDir:      DefaultGitHubWorkflowDir,
		githubWorkflowFilename: DefaultGitHubWorkflowFilename,
	}
	want := filepath.Join(DefaultGitHubWorkflowDir, DefaultGitHubWorkflowFilename)
	assert.Equal(t, cc.OutputPath(), want)
}

func TestCIConfig_FnGitHubWorkflowFilepath(t *testing.T) {
	root := "/home/user/myfunc"
	cc := CIConfig{
		fnRoot:                 root,
		githubWorkflowDir:      DefaultGitHubWorkflowDir,
		githubWorkflowFilename: DefaultGitHubWorkflowFilename,
	}
	want := filepath.Join(root, DefaultGitHubWorkflowDir, DefaultGitHubWorkflowFilename)
	assert.Equal(t, cc.FnGitHubWorkflowFilepath(), want)
}

// ---------------------------------------------------------------------------
// resolveBuilder — the main untested code block from config.go
// ---------------------------------------------------------------------------

func TestResolveBuilder(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		remote  bool
		want    string
		wantErr bool
	}{
		{name: "go local", runtime: "go", remote: false, want: "host"},
		{name: "go remote", runtime: "go", remote: true, want: "pack"},
		{name: "node local", runtime: "node", remote: false, want: "pack"},
		{name: "node remote", runtime: "node", remote: true, want: "pack"},
		{name: "typescript local", runtime: "typescript", remote: false, want: "pack"},
		{name: "typescript remote", runtime: "typescript", remote: true, want: "pack"},
		{name: "rust local", runtime: "rust", remote: false, want: "pack"},
		{name: "rust remote", runtime: "rust", remote: true, want: "pack"},
		{name: "quarkus local", runtime: "quarkus", remote: false, want: "pack"},
		{name: "quarkus remote", runtime: "quarkus", remote: true, want: "pack"},
		{name: "springboot local", runtime: "springboot", remote: false, want: "pack"},
		{name: "springboot remote", runtime: "springboot", remote: true, want: "pack"},
		{name: "python local", runtime: "python", remote: false, want: "host"},
		{name: "python remote", runtime: "python", remote: true, want: "s2i"},
		{name: "unknown runtime", runtime: "fortran", remote: false, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveBuilder(tc.runtime, tc.remote)
			if tc.wantErr {
				assert.Assert(t, err != nil, "expected an error for runtime %q", tc.runtime)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// Additional simple bool/string accessors
// ---------------------------------------------------------------------------

func TestCIConfig_BoolAccessors(t *testing.T) {
	cc := CIConfig{
		registryLogin:    true,
		selfHostedRunner: true,
		remoteBuild:      true,
		workflowDispatch: true,
		testStep:         true,
		force:            true,
	}
	assert.Assert(t, cc.RegistryLogin())
	assert.Assert(t, cc.SelfHostedRunner())
	assert.Assert(t, cc.RemoteBuild())
	assert.Assert(t, cc.WorkflowDispatch())
	assert.Assert(t, cc.TestStep())
	assert.Assert(t, cc.Force())
}

func TestCIConfig_StringAccessors(t *testing.T) {
	cc := CIConfig{
		branch:              "feature-x",
		workflowName:        "My Deploy",
		kubeconfigSecret:    "MY_KUBECONFIG",
		registryLoginUrlVar: "MY_REGISTRY_URL",
		registryUserVar:     "MY_USER",
		registryPassSecret:  "MY_PASS",
		registryUrlVar:      "MY_REG",
	}
	assert.Equal(t, cc.Branch(), "feature-x")
	assert.Equal(t, cc.WorkflowName(), "My Deploy")
	assert.Equal(t, cc.KubeconfigSecret(), "MY_KUBECONFIG")
	assert.Equal(t, cc.RegistryLoginUrlVar(), "MY_REGISTRY_URL")
	assert.Equal(t, cc.RegistryUserVar(), "MY_USER")
	assert.Equal(t, cc.RegistryPassSecret(), "MY_PASS")
	assert.Equal(t, cc.RegistryUrlVar(), "MY_REG")
}
