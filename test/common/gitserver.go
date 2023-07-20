package common

import (
	"context"

	"strings"
	"testing"

	"knative.dev/func/pkg/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetGitServer(T *testing.T) GitProvider {
	gitTestServer := GitTestServerProvider{}
	gitTestServer.Init(T)
	return &gitTestServer
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

type GitTestServerProvider struct {
	PodName    string
	ServiceUrl string
	Kubectl    *TestExecCmd
	t          *testing.T
}

func (g *GitTestServerProvider) Init(T *testing.T) {

	g.t = T
	if g.PodName == "" {
		config, err := k8s.GetClientConfig().ClientConfig()
		if err != nil {
			T.Fatal(err.Error())
		}
		clientSet, err := kubernetes.NewForConfig(config)
		if err != nil {
			T.Fatal(err.Error())
		}
		ctx := context.Background()

		namespace, _, _ := k8s.GetClientConfig().Namespace()
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "serving.knative.dev/service=func-git",
		})
		if err != nil {
			T.Fatal(err.Error())
		}
		for _, pod := range podList.Items {
			g.PodName = pod.Name
		}
	}

	if g.ServiceUrl == "" {
		// Get Route Name
		_, g.ServiceUrl = GetKnativeServiceRevisionAndUrl(T, "func-git")
	}

	if g.Kubectl == nil {
		g.Kubectl = &TestExecCmd{
			Binary:              "kubectl",
			ShouldDumpCmdLine:   true,
			ShouldDumpOnSuccess: true,
			T:                   T,
		}
	}
	T.Logf("Initialized HTTP Func Git Server: Server URL = %v Pod Name = %v\n", g.ServiceUrl, g.PodName)
}

func (g *GitTestServerProvider) CreateRepository(repoName string) *GitRemoteRepo {
	// kubectl exec $podname -c user-container -- git-repo create $reponame
	cmdResult := g.Kubectl.Exec("exec", g.PodName, "-c", "user-container", "--", "git-repo", "create", repoName)
	if !strings.Contains(cmdResult.Out, "created") {
		g.t.Fatal("unable to create git bare repository " + repoName)
	}
	namespace, _, _ := k8s.GetClientConfig().Namespace()
	gitRepo := &GitRemoteRepo{
		RepoName:         repoName,
		ExternalCloneURL: g.ServiceUrl + "/" + repoName + ".git",
		ClusterCloneURL:  "http://func-git." + namespace + ".svc.cluster.local/" + repoName + ".git",
	}
	return gitRepo
}

func (g *GitTestServerProvider) DeleteRepository(repoName string) {
	cmdResult := g.Kubectl.Exec("exec", g.PodName, "-c", "user-container", "--", "git-repo", "delete", repoName)
	if !strings.Contains(cmdResult.Out, "deleted") {
		g.t.Fatal("unable to delete git bare repository " + repoName)
	}
}
