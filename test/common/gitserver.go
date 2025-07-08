package common

import (
	"context"
	"fmt"

	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	Kubectl      *TestExecCmd
	namespace    string
	externalHost string
	t            *testing.T
}

// name of the pod,svc and ingress
const podName = "func-git"
const svcName = podName
const ingressName = podName

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
	if g.namespace == "" {
		g.namespace, _, _ = k8s.GetClientConfig().Namespace()
	}

	if g.externalHost == "" {
		cli, err := k8s.NewKubernetesClientset()
		if err != nil {
			t.Fatal(err)
		}
		i, err := cli.NetworkingV1().Ingresses(g.namespace).Get(context.Background(), ingressName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		g.externalHost = i.Spec.Rules[0].Host
	}

	t.Logf("Initialized HTTP Func Git Server: Server URL = %s Pod Name = %s\n", g.externalHost, podName)
}

func (g *GitTestServerKnativeProvider) CreateRepository(repoName string) *GitRemoteRepo {
	cmdResult := g.Kubectl.Exec("exec", podName, "--", "git-repo", "create", repoName)
	if !strings.Contains(cmdResult.Out, "created") {
		g.t.Fatal("unable to create git bare repository " + repoName)
	}
	gitRepo := &GitRemoteRepo{
		RepoName:         repoName,
		ExternalCloneURL: fmt.Sprintf("http://%s/%s.git", g.externalHost, repoName),
		ClusterCloneURL:  fmt.Sprintf("http://%s.%s.svc.cluster.local/%s.git", svcName, g.namespace, repoName),
	}
	return gitRepo
}

func (g *GitTestServerKnativeProvider) DeleteRepository(repoName string) {
	cmdResult := g.Kubectl.Exec("exec", podName, "--", "git-repo", "delete", repoName)
	if !strings.Contains(cmdResult.Out, "deleted") {
		g.t.Fatal("unable to delete git bare repository " + repoName)
	}
}
