//go:build integration
// +build integration

package knative_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	fn "knative.dev/func"
	"knative.dev/func/k8s"
	"knative.dev/func/knative"
)

func TestIntegration(t *testing.T) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	namespace := "knative-integration-test-ns-" + rand.String(5)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Spec: corev1.NamespaceSpec{},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cliSet.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}) })
	t.Log("created namespace: ", namespace)

	minScale := int64(3)
	maxScale := int64(100)
	varName := "FUNC_TEST_PWD"
	varVal := "nbusr123"

	functionName := "fn"
	function := fn.Function{
		SpecVersion: "SNAPSHOT",
		Root:        "/non/existent",
		Name:        functionName,
		Runtime:     "blub",
		Template:    "cloudevents",
		Image:       "quay.io/mvasek/func-test-service",
		ImageDigest: "sha256:85d199814a09e68b2f7e4ca2ff049531c6397e903ae558347aba5d94dd24fb07",
		Created:     time.Time{},
		Deploy: fn.DeploySpec{
			Namespace: namespace,
			Options: fn.Options{
				Scale: &fn.ScaleOptions{
					Min: &minScale,
					Max: &maxScale,
				},
			},
		},
		Run: fn.RunSpec{Envs: []fn.Env{
			{Name: &varName, Value: &varVal},
		}},
	}

	var buff = &buffer{}
	now := time.Now()
	go func() {
		_ = knative.GetKServiceLogs(ctx, namespace, functionName, function.ImageWithDigest(), &now, buff)
	}()

	deployer := knative.NewDeployer(knative.WithDeployerNamespace(namespace), knative.WithDeployerVerbose(false))

	depRes, err := deployer.Deploy(ctx, function)
	if err != nil {
		t.Fatal(err)
	}

	outStr := buff.String()
	t.Logf("deploy result: %+v", depRes)
	t.Log("out:\n" + outStr)

	if strings.Count(outStr, "starting app") < int(minScale) {
		t.Errorf("application should be scaled at least to %d pods", minScale)
	}

	if !strings.Contains(outStr, varName+"="+varVal) {
		t.Error("runtime environment variable not propagated")
	}

	describer := knative.NewDescriber(namespace, false)
	instance, err := describer.Describe(ctx, functionName)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("instance: %+v", instance)

	lister := knative.NewLister(namespace, false)
	list, err := lister.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("functions list: %+v", list)

	if len(list) != 1 {
		t.Errorf("expected exactly one functions but got: %d", len(list))
	} else {
		if list[0].URL != instance.Route {
			t.Error("URL mismatch")
		}
	}

	remover := knative.NewRemover(namespace, false)
	err = remover.Remove(ctx, functionName)
	if err != nil {
		t.Fatal(err)
	}

	list, err = lister.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 0 {
		t.Errorf("expected exactly zero functions but got: %d", len(list))
	}
}

type buffer struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *buffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
