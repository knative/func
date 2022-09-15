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

	"knative.dev/kn-plugin-func/k8s"
	. "knative.dev/kn-plugin-func/testing"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
)

func TestDialInClusterService(t *testing.T) {
	defer WithEnvVar(t, "SOCAT_IMAGE", "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root")()

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

	nginxPod := &coreV1.Pod{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        "dialer-test-pod",
			Labels:      map[string]string{"app": "dialer-test-app"},
			Annotations: nil,
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{
				{
					Name:  "dialer-testing-nginx",
					Image: "nginx",
				},
			},
			DNSPolicy:     coreV1.DNSClusterFirst,
			RestartPolicy: coreV1.RestartPolicyNever,
		},
	}

	_, err = cliSet.CoreV1().Pods(testingNS.Name).Create(ctx, nginxPod, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cliSet.CoreV1().Pods(testingNS.Name).Delete(ctx, nginxPod.Name, deleteOpts)
	})
	t.Log("created pod: ", nginxPod.Name)

	nginxService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "dialer-test-service",
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:       "http",
					Protocol:   coreV1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
			Selector: map[string]string{
				"app": "dialer-test-app",
			},
		},
	}

	nginxService, err = cliSet.CoreV1().Services(testingNS.Name).Create(ctx, nginxService, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cliSet.CoreV1().Services(testingNS.Name).Delete(ctx, nginxService.Name, deleteOpts)
	})
	t.Log("created svc: ", nginxService.Name)

	// wait for service to start
	time.Sleep(time.Second * 10)

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

	svcInClusterURL := fmt.Sprintf("http://%s.%s.svc", nginxService.Name, nginxService.Namespace)
	resp, err := client.Get(svcInClusterURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	runeReader := bufio.NewReader(resp.Body)
	matched, err := regexp.MatchReader("Welcome to nginx!", runeReader)
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
	defer WithEnvVar(t, "SOCAT_IMAGE", "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root")()

	var ctx = context.Background()

	dialer, err := k8s.NewInClusterDialer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		dialer.Close()
	})

	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}

	var client = http.Client{
		Transport: transport,
	}

	_, err = client.Get("http://does-not.exists.svc")
	if err == nil {
		t.Error("error was expected but got nil")
		return
	}
	if !strings.Contains(err.Error(), "not resolve") {
		t.Errorf("error %q doesn't containe expected sub-string: ", err.Error())
	}
}
