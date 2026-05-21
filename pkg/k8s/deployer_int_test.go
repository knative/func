//go:build integration

package k8s_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func defaultKc() *k8s.Client {
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	return k8s.NewClient(cc)
}

func TestInt_FullPath(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_FullPath(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewLister(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Deploy(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Metadata(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_Events(t *testing.T) {
	t.Skip("Kubernetes deploy does not support func subscribe yet")

	kc := defaultKc()
	deployertesting.TestInt_Events(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_Scale(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Scale(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_EnvsUpdate(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_ResourceValidationOnFirstDeploy(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_ResourceValidationOnFirstDeploy(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}

func TestInt_OperatorSync(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_OperatorSync(t,
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(kc, false),
		k8s.NewDescriber(kc, false),
		k8s.KubernetesDeployerName)
}
