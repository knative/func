//go:build integration

package knative_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
)

func defaultKc() *k8s.Client {
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	return k8s.NewClient(cc)
}

func TestInt_FullPath(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_FullPath(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, true),
		knative.NewLister(kc, true),
		knative.NewDescriber(kc, true),
		knative.KnativeDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Deploy(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Metadata(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_Events(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Events(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_Scale(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_Scale(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_EnvsUpdate(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_ResourceValidationOnFirstDeploy(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_ResourceValidationOnFirstDeploy(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}

func TestInt_OperatorSync(t *testing.T) {
	kc := defaultKc()
	deployertesting.TestInt_OperatorSync(t,
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, false),
		knative.NewDescriber(kc, false),
		knative.KnativeDeployerName)
}
