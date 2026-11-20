package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

func TestDelete_LegacyDeployerForRedeploy(t *testing.T) {
	for _, tt := range []struct {
		name           string
		version        string
		legacyDeployer string
		globalDeployer string
		flagDeployer   string
		envDeployer    string
		skipDelete     bool
		want           string
	}{
		{name: "0.38.0 keda", version: "0.38.0", legacyDeployer: "keda", want: "keda"},
		{name: "0.36.0 keda", version: "0.36.0", legacyDeployer: "keda", want: "keda"},
		{name: "raw", version: "0.38.0", legacyDeployer: "raw", want: "raw"},
		{name: "knative", version: "0.38.0", legacyDeployer: "knative", want: "knative"},
		{name: "intent beats global default", version: "0.38.0", legacyDeployer: "keda", globalDeployer: "raw", want: "keda"},
		{name: "explicit flag overrides", version: "0.38.0", legacyDeployer: "keda", flagDeployer: "raw", want: "raw"},
		{name: "environment overrides", version: "0.38.0", legacyDeployer: "keda", envDeployer: "raw", want: "raw"},
		{name: "plain redeploy control", version: "0.38.0", legacyDeployer: "keda", skipDelete: true, want: "keda"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := FromTempDirectory(t)
			f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry})
			if err != nil {
				t.Fatal(err)
			}
			legacy := fmt.Sprintf("specVersion: %s\nname: %s\nruntime: go\nnamespace: myns\nregistry: %s\ncreated: %s\ndeploy:\n  namespace: myns\n  deployer: %s\n",
				tt.version, f.Name, TestRegistry, f.Created.UTC().Format(time.RFC3339), tt.legacyDeployer)
			if err := os.WriteFile(filepath.Join(root, fn.FunctionFile), []byte(legacy), 0644); err != nil {
				t.Fatal(err)
			}
			if tt.globalDeployer != "" {
				if err := config.CreatePaths(); err != nil {
					t.Fatal(err)
				}
				if err := (config.Global{Deployer: tt.globalDeployer}).Write(config.File()); err != nil {
					t.Fatal(err)
				}
			}
			if !tt.skipDelete {
				remover := mock.NewRemover()
				remover.RemoveFn = func(name, namespace string) error {
					if name != f.Name || namespace != "myns" {
						t.Errorf("remove target = %s/%s, want myns/%s", namespace, name, f.Name)
					}
					return nil
				}
				deleteCmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
				deleteCmd.SetArgs([]string{})
				if err := deleteCmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if !remover.RemoveInvoked {
					t.Fatal("remover not invoked")
				}
				loaded, err := fn.NewFunction(root)
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Deployer != tt.legacyDeployer || loaded.Deploy.Deployer != "" || loaded.Deploy.Namespace != "" {
					t.Errorf("after delete: intent=%q observed=%q namespace=%q", loaded.Deployer, loaded.Deploy.Deployer, loaded.Deploy.Namespace)
				}
				if loaded.SpecVersion != fn.LastSpecVersion() {
					t.Errorf("persisted specVersion = %q, want %q", loaded.SpecVersion, fn.LastSpecVersion())
				}
			}
			if tt.envDeployer != "" {
				t.Setenv("FUNC_DEPLOYER", tt.envDeployer)
			}
			deployer := mock.NewDeployer()
			deploy := deployer.DeployFn
			deployer.DeployFn = func(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
				if f.Deployer != tt.want {
					t.Errorf("deployer received intent %q, want %q", f.Deployer, tt.want)
				}
				return deploy(ctx, f)
			}
			deployCmd := NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
			args := []string{}
			if tt.flagDeployer != "" {
				args = append(args, "--deployer", tt.flagDeployer)
			}
			deployCmd.SetArgs(args)
			if err := deployCmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !deployer.DeployInvoked {
				t.Fatal("deployer not invoked")
			}
			loaded, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Deployer != tt.want || loaded.Deploy.Deployer != tt.want {
				t.Errorf("after redeploy: intent=%q observed=%q, want %q", loaded.Deployer, loaded.Deploy.Deployer, tt.want)
			}
		})
	}
}
