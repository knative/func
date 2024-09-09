package common

import (
	"fmt"
	"os"

	"strings"
	"testing"
)

// ------------------------------------------------------
// Git Server used by Openshift CI. It is deployed os a
// regular POD with an exposed Route URL.
// ------------------------------------------------------

type GitTestServerOpenshiftCI struct {
	PodName  string
	RouteURL string
	Kubectl  *TestExecCmd
	t        *testing.T
}

var gitServerPodName string
var gitServerRouteURL string

func init() {
	// Openshift CI runs `openshift/e2e_oncluster_test.sh` which sets these env vars
	gitServerPodName = os.Getenv("E2E_GIT_SERVER_PODNAME")
	gitServerRouteURL = os.Getenv("E2E_GIT_SERVER_ROUTE_URL")
	if gitServerPodName != "" && gitServerRouteURL != "" {
		DefaultGitServer = &GitTestServerOpenshiftCI{}
		fmt.Println("Setting default test git server for CI")
	}
}

func (g *GitTestServerOpenshiftCI) Init(T *testing.T) {

	g.PodName = gitServerPodName
	g.RouteURL = gitServerRouteURL
	g.t = T
	if g.Kubectl == nil {
		g.Kubectl = &TestExecCmd{
			Binary:              "oc",
			ShouldDumpCmdLine:   true,
			ShouldDumpOnSuccess: true,
			T:                   T,
		}
	}

	T.Logf("Initialized HTTP Func Git Server: Server URL = %v Pod Name = %v\n", g.RouteURL, g.PodName)
}

func (g *GitTestServerOpenshiftCI) CreateRepository(repoName string) *GitRemoteRepo {
	// kubectl exec gitserver -- git-repo create $reponame
	cmdResult := g.Kubectl.Exec("exec", g.PodName, "--", "git-repo", "create", repoName)

	if !strings.Contains(cmdResult.Out, "created") {
		g.t.Fatal("unable to create git bare repository " + repoName)
	}
	gitRepo := &GitRemoteRepo{
		RepoName:         repoName,
		ExternalCloneURL: g.RouteURL + "/" + repoName + ".git",
		ClusterCloneURL:  g.RouteURL + "/" + repoName + ".git",
	}
	return gitRepo
}

func (g *GitTestServerOpenshiftCI) DeleteRepository(repoName string) {
	cmdResult := g.Kubectl.Exec("exec", g.PodName, "--", "git-repo", "delete", repoName)
	if !strings.Contains(cmdResult.Out, "deleted") {
		g.t.Fatal("unable to delete git bare repository " + repoName)
	}
}
