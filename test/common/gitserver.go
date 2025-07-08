package common

import (
	"fmt"

	"strings"
	"testing"

	"knative.dev/func/pkg/k8s"
)

var DefaultGitServer GitProvider

func GetGitServer(T *testing.T) GitProvider {
	if DefaultGitServer == nil {
		DefaultGitServer = &GitTestServerKnativeProvider{}
	}
	DefaultGitServer.Init(T)
	return DefaultGitServer
}

type GitRemoteRepo struct {
	RepoName         string
	ExternalCloneURL string
	ClusterCloneURL  string
}

type GitProvider interface {
	Init(T *testing.T)
	CreateRepository(repoName string) *GitRemoteRepo
	DeleteRepository(repoName string)
}

// ------------------------------------------------------
// Git Server on Kubernetes as Knative Service (func-git)
// ------------------------------------------------------

type GitTestServerKnativeProvider struct {
	Kubectl *TestExecCmd
	ns      string
	t       *testing.T
}

const podName = "func-git"

func (g *GitTestServerKnativeProvider) Init(t *testing.T) {
	g.t = t
	if g.Kubectl == nil {
		g.Kubectl = &TestExecCmd{
			Binary:              "kubectl",
			ShouldDumpCmdLine:   true,
			ShouldDumpOnSuccess: true,
			T:                   t,
		}
	}
	if g.ns == "" {
		g.ns, _, _ = k8s.GetClientConfig().Namespace()
	}
	t.Logf("Initialized HTTP Func Git Server: Server URL = func-git.%s.localtest.me Pod Name = %s\n", g.ns, podName)
}

func (g *GitTestServerKnativeProvider) CreateRepository(repoName string) *GitRemoteRepo {
	cmdResult := g.Kubectl.Exec("exec", podName, "--", "git-repo", "create", repoName)
	if !strings.Contains(cmdResult.Out, "created") {
		g.t.Fatal("unable to create git bare repository " + repoName)
	}
	gitRepo := &GitRemoteRepo{
		RepoName:         repoName,
		ExternalCloneURL: fmt.Sprintf("http://func-git.%s.localtest.me/%s.git", g.ns, repoName),
		ClusterCloneURL:  fmt.Sprintf("http://func-git.%s.svc.cluster.local/%s.git", g.ns, repoName),
	}
	return gitRepo
}

func (g *GitTestServerKnativeProvider) DeleteRepository(repoName string) {
	cmdResult := g.Kubectl.Exec("exec", podName, "--", "git-repo", "delete", repoName)
	if !strings.Contains(cmdResult.Out, "deleted") {
		g.t.Fatal("unable to delete git bare repository " + repoName)
	}
}
