//go:build integration
// +build integration

package k8s_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/k8s"
)

func TestDialInClusterService(t *testing.T) {
	var err error
	var ctx = context.Background()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	creatOpts := metaV1.CreateOptions{}
	deleteOpts := metaV1.DeleteOptions{}

	testingNS := &coreV1.Namespace{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "dialer-test-ns-" + rand.String(5),
		},
		Spec: coreV1.NamespaceSpec{},
	}

	_, err = cliSet.CoreV1().Namespaces().Create(ctx, testingNS, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cliSet.CoreV1().Namespaces().Delete(ctx, testingNS.Name, deleteOpts)
	})
	t.Log("created namespace: ", testingNS.Name)

	one := int32(1)
	labels := map[string]string{"app.kubernetes.io/name": "helloworld"}
	deployment := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:   "helloworld-" + rand.String(5),
			Labels: labels,
		},
		Spec: appsV1.DeploymentSpec{
			Replicas: &one,
			Selector: &metaV1.LabelSelector{
				MatchLabels: labels,
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: labels,
				},
				Spec: coreV1.PodSpec{
					Containers: []coreV1.Container{
						{
							Name:  "helloworld",
							Image: "gcr.io/knative-samples/helloworld-go@sha256:2babda8ec819e24d5a6342095e8f8a25a67b44eb7231ae253ecc2c448632f07e",
							Ports: []coreV1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      coreV1.ProtocolTCP,
								},
							},
							Env: []coreV1.EnvVar{
								{
									Name:  "PORT",
									Value: "8080",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = cliSet.AppsV1().Deployments(testingNS.Name).Create(ctx, deployment, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.AppsV1().Deployments(testingNS.Name).Delete(ctx, deployment.Name, deleteOpts)
	})
	t.Log("created deployment:", deployment.Name)

	svc := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "helloworld",
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:       "http",
					Protocol:   coreV1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
			Selector: labels,
		},
	}

	svc, err = cliSet.CoreV1().Services(testingNS.Name).Create(ctx, svc, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.CoreV1().Services(testingNS.Name).Delete(ctx, svc.Name, deleteOpts)
	})
	t.Log("created svc:", svc.Name)

	// wait for service to start
	time.Sleep(time.Second * 5)

	dialer := k8s.NewLazyInitInClusterDialer()
	t.Cleanup(func() {
		dialer.Close()
	})

	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}

	var client = http.Client{
		Transport: transport,
	}

	svcInClusterURL := fmt.Sprintf("http://%s.%s.svc", svc.Name, svc.Namespace)
	resp, err := client.Get(svcInClusterURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	runeReader := bufio.NewReader(resp.Body)
	matched, err := regexp.MatchReader("Hello World!", runeReader)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Error("body doesn't contain 'Welcome to nginx!' substring")
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(svcInClusterURL)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
		}()
	}
	wg.Wait()
}

func TestDialUnreachable(t *testing.T) {
	var ctx = context.Background()

	dialer, err := k8s.NewInClusterDialer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		dialer.Close()
	})

	_, err = dialer.DialContext(ctx, "tcp", "does-not.exists.svc:80")
	if err == nil {
		t.Error("error was expected but got nil")
		return
	}
	if !strings.Contains(err.Error(), "no such host") {
		t.Errorf("error %q doesn't contain expected substring: ", err.Error())
	}

	_, err = dialer.DialContext(ctx, "tcp", "localhost:80")
	if err == nil {
		t.Error("error was expected but got nil")
		return
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error %q doesn't contain expected substring: ", err.Error())
	}
}
