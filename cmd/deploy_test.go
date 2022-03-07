package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/ory/viper"
	"k8s.io/utils/pointer"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

func Test_runDeploy(t *testing.T) {
	tests := []struct {
		name                 string
		gitURL               string
		gitBranch            string
		gitDir               string
		buildType            string
		funcFile             string
		expectFileURL        *string
		expectFileBranch     *string
		expectFileContextDir *string
		expectCallURL        *string
		expectCallBranch     *string
		expectCallContextDir *string
		errString            string
	}{
		{
			name:      "Git arguments don't get saved to func.yaml but are used in the pipeline invocation",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git",
			gitBranch: "main",
			gitDir:    "func",
			buildType: fn.BuildTypeGit,
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
		},
		{
			name:      "Git url gets split when in the format url#branch",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git#main",
			gitDir:    "func",
			buildType: fn.BuildTypeGit,
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
		},
		{
			name:      "Git arguments override func.yaml but don't get saved",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git",
			gitBranch: "main",
			gitDir:    "func",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00
build: git
git:
  url: git@github.com:my-repo/my-function.git
  revision: master
  contextDir: pwd`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
			expectFileURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectFileBranch:     pointer.StringPtr("master"),
			expectFileContextDir: pointer.StringPtr("pwd"),
		},
		{
			name: "Git properties work without arguments",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00
build: git
git:
  url: git@github.com:my-repo/my-function.git
  revision: master
  contextDir: pwd`,
			expectFileURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectFileBranch:     pointer.StringPtr("master"),
			expectFileContextDir: pointer.StringPtr("pwd"),
			expectCallURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectCallBranch:     pointer.StringPtr("master"),
			expectCallContextDir: pointer.StringPtr("pwd"),
		},
		{
			name:      "check error when providing git flags with buildType local",
			gitURL:    "git@github.com:my-repo/my-function.git",
			buildType: "local",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			errString: "remote git arguments require the --build=git flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captureFn fn.Function
			pipeline := &mock.PipelinesProvider{
				RunFn: func(f fn.Function) error {
					captureFn = f
					return nil
				},
			}
			deployer := mock.NewDeployer()
			defer fromTempDir(t)()
			cmd := NewDeployCmd(
				fn.WithPipelinesProvider(pipeline),
				fn.WithDeployer(deployer),
			)

			viper.SetDefault("git-url", tt.gitURL)
			viper.SetDefault("git-branch", tt.gitBranch)
			viper.SetDefault("git-dir", tt.gitDir)
			viper.SetDefault("build", tt.buildType)
			viper.SetDefault("registry", "docker.io/tigerteam")

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.TODO()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				if tt.errString == "" {
					t.Fatalf("Problem executing command: %v", err)
				} else if err := err.Error(); tt.errString != err {
					t.Fatalf("Error expected to be %v but was %v", tt.errString, err)
				}
			}

			fileFunction, err := fn.NewFunction(".")

			if err != nil {
				t.Fatalf("problem creating function: %v", err)
			}

			{
				if fileURL, expectedURL := pointer.StringPtrDerefOr(fileFunction.Git.URL, ""), pointer.StringPtrDerefOr(tt.expectFileURL, ""); fileURL != expectedURL {
					t.Fatalf("file Git URL expected to be (%v) but was (%v)", expectedURL, fileURL)
				}
				if fileBranch, expectedBranch := pointer.StringPtrDerefOr(fileFunction.Git.Revision, ""), pointer.StringPtrDerefOr(tt.expectFileBranch, ""); fileBranch != expectedBranch {
					t.Fatalf("file Git branch expected to be (%v) but was (%v)", expectedBranch, fileBranch)
				}
				if fileDir, expectedDir := pointer.StringPtrDerefOr(fileFunction.Git.ContextDir, ""), pointer.StringPtrDerefOr(tt.expectFileContextDir, ""); fileDir != expectedDir {
					t.Fatalf("file Git contextDir expected to be (%v) but was (%v)", expectedDir, fileDir)
				}
			}

			{
				if caputureURL, expectedURL := pointer.StringPtrDerefOr(captureFn.Git.URL, ""), pointer.StringPtrDerefOr(tt.expectCallURL, ""); caputureURL != expectedURL {
					t.Fatalf("call Git URL expected to be (%v) but was (%v)", expectedURL, caputureURL)
				}
				if captureBranch, expectedBranch := pointer.StringPtrDerefOr(captureFn.Git.Revision, ""), pointer.StringPtrDerefOr(tt.expectCallBranch, ""); captureBranch != expectedBranch {
					t.Fatalf("call Git Branch expected to be (%v) but was (%v)", expectedBranch, captureBranch)
				}
				if captureDir, expectedDir := pointer.StringPtrDerefOr(captureFn.Git.ContextDir, ""), pointer.StringPtrDerefOr(tt.expectCallContextDir, ""); captureDir != expectedDir {
					t.Fatalf("call Git Dir expected to be (%v) but was (%v)", expectedDir, captureDir)
				}
			}
		})
	}
}
