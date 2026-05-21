//go:build integration

package k8s_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// TestDialInClusterService ensures that dialer is able to establish HTTP
// connections to services only accessible in-cluster.
//
// The InClusterDialer allows access to internal services from outside the
// cluster by creating a temporary socat pod inside the cluster which is used
// as a TCP proxy/tunnel via kubectl exec.
func TestInt_DialInClusterService(t *testing.T) {
	var err error
	var ctx = t.Context()

	// Initialize client configuration from kubeconfig or in-cluster config
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)

	// Create a clientset for API operations
	cliSet, err := kc.Clientset()
	if err != nil {
		t.Fatal(err)
	}

	// Configure resource cleanup options - Foreground deletion ensures pods
	// are deleted before the deployment/service is removed
	pp := metaV1.DeletePropagationForeground
	creatOpts := metaV1.CreateOptions{}
	deleteOpts := metaV1.DeleteOptions{
		PropagationPolicy: &pp,
	}

	// Determine which namespace to use for test resources
	testingNS, err := kc.DefaultNamespace()
	if err != nil {
		t.Fatal(err)
	}

	// Generate a random suffix to avoid conflicts with parallel test runs
	rnd := rand.String(5)
	one := int32(1)
	labels := map[string]string{"app.kubernetes.io/name": "helloworld"}

	// Create a simple HTTP server deployment using the Knative hello-world
	// sample.
	deployment := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:   "helloworld-" + rnd,
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
							Name: "helloworld",
							// Using a specific SHA ensures test stability - this image is a simple
							// HTTP server that returns "Hello World!" responses
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

	// Deploy the hello-world server to the cluster
	_, err = cliSet.AppsV1().Deployments(testingNS).Create(ctx, deployment, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.AppsV1().Deployments(testingNS).Delete(ctx, deployment.Name, deleteOpts)
	})
	t.Log("created deployment:", deployment.Name)

	// Create a Service to expose the deployment within the cluster.
	// The service maps port 80 -> 8080 (container port) and uses label selectors
	// to route traffic to the deployment's pods.
	svc := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "helloworld-" + rnd,
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:       "http",
					Protocol:   coreV1.ProtocolTCP,
					Port:       80,                   // Service port (what clients connect to)
					TargetPort: intstr.FromInt(8080), // Pod port (where container listens)
				},
			},
			Selector: labels,
		},
	}

	// Create the service in the cluster
	svc, err = cliSet.CoreV1().Services(testingNS).Create(ctx, svc, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.CoreV1().Services(testingNS).Delete(ctx, svc.Name, deleteOpts)
	})
	t.Log("created svc:", svc.Name)

	// Wait for the deployment pods to be ready
	if err := k8s.WaitForDeploymentAvailable(ctx, cliSet, testingNS, deployment.Name, 60*time.Second); err != nil {
		t.Fatal("deployment never became ready:", err)
	}

	// Initialize the InClusterDialer. This will create a socat pod in the
	// cluster that acts as a TCP proxy, allowing us to reach cluster-internal
	// services. The "lazy init" variant only creates the pod when first used.
	dialer := k8s.NewLazyInitInClusterDialer(kc)
	t.Cleanup(func() {
		dialer.Close()
	})

	// Configure HTTP client to use our custom dialer for all connections.
	// This routes HTTP requests: client -> kubectl exec -> socat pod -> service
	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}

	var client = http.Client{
		Transport: transport,
	}

	// Construct the cluster-internal DNS name for the service.
	// Format: <service-name>.<namespace>.svc (.cluster.local suffix optional)
	svcInClusterURL := fmt.Sprintf("http://%s.%s.svc", svc.Name, svc.Namespace)

	// Make an HTTP GET request through the dialer tunnel to the internal service
	resp, err := client.Get(svcInClusterURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Verify the response contains the expected "Hello World!" message
	runeReader := bufio.NewReader(resp.Body)
	matched, err := regexp.MatchReader("Hello World!", runeReader)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		// Note: Error message mentions nginx but we're testing hello-world
		t.Error("body doesn't contain 'Hello World!' substring")
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Stress test: Make 10 concurrent requests to verify the dialer handles
	// multiple simultaneous connections correctly. This tests the stability
	// of the kubectl exec tunnel under load.
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			resp, err := client.Get(svcInClusterURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			// Fully consume the response body to complete the HTTP transaction
			_, err = io.Copy(io.Discard, resp.Body)
			return err
		})
	}
	err = eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestInt_DialUnreachable(t *testing.T) {
	var ctx = t.Context()

	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)
	dialer, err := k8s.NewInClusterDialer(ctx, kc)
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
	if !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "does not resolve") {
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

// TestInt_DialContextExpiry verifies that a connection established via
// DialContext continues to work after the dial context has expired.
// Per net.Dialer.DialContext semantics: "Once successfully connected, any
// expiration of the context will not affect the connection."
//
// We use the raw net.Conn returned by DialContext and perform an HTTP
// request "by hand" so that only the dial step is governed by dialCtx.
// (http.Client uses the request context for the entire request lifetime,
// which would conflate what we are testing.)
func TestInt_DialContextExpiry(t *testing.T) {
	var setupCtx = t.Context()

	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)
	cliSet, err := kc.Clientset()
	if err != nil {
		t.Fatal(err)
	}

	pp := metaV1.DeletePropagationForeground
	creatOpts := metaV1.CreateOptions{}
	deleteOpts := metaV1.DeleteOptions{PropagationPolicy: &pp}

	testingNS, err := kc.DefaultNamespace()
	if err != nil {
		t.Fatal(err)
	}

	rnd := rand.String(5)
	one := int32(1)
	labels := map[string]string{"app.kubernetes.io/name": "helloworld-ctx"}

	deployment := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{Name: "helloworld-ctx-" + rnd, Labels: labels},
		Spec: appsV1.DeploymentSpec{
			Replicas: &one,
			Selector: &metaV1.LabelSelector{MatchLabels: labels},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{Labels: labels},
				Spec: coreV1.PodSpec{
					Containers: []coreV1.Container{{
						Name:  "helloworld",
						Image: "gcr.io/knative-samples/helloworld-go@sha256:2babda8ec819e24d5a6342095e8f8a25a67b44eb7231ae253ecc2c448632f07e",
						Ports: []coreV1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: coreV1.ProtocolTCP}},
						Env:   []coreV1.EnvVar{{Name: "PORT", Value: "8080"}},
					}},
				},
			},
		},
	}
	_, err = cliSet.AppsV1().Deployments(testingNS).Create(setupCtx, deployment, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cliSet.AppsV1().Deployments(testingNS).Delete(setupCtx, deployment.Name, deleteOpts) })

	svc := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "helloworld-ctx-" + rnd},
		Spec: coreV1.ServiceSpec{
			Ports:    []coreV1.ServicePort{{Name: "http", Protocol: coreV1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(8080)}},
			Selector: labels,
		},
	}
	svc, err = cliSet.CoreV1().Services(testingNS).Create(setupCtx, svc, creatOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cliSet.CoreV1().Services(testingNS).Delete(setupCtx, svc.Name, deleteOpts) })

	if err := k8s.WaitForDeploymentAvailable(setupCtx, cliSet, testingNS, deployment.Name, 60*time.Second); err != nil {
		t.Fatal("deployment never became ready:", err)
	}

	// Create the dialer pod eagerly so that pod creation is not tied to dialCtx.
	dialer, err := k8s.NewInClusterDialer(setupCtx, kc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dialer.Close() })

	// Use a short-lived context for dial establishment only.
	dialCtx, dialCancel := context.WithTimeout(setupCtx, 30*time.Second)

	// Dial directly to obtain a raw net.Conn.
	addr := fmt.Sprintf("%s.%s.svc:80", svc.Name, svc.Namespace)
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		dialCancel()
		t.Fatal(err)
	}
	t.Log("connection established while dial context is alive")

	// Cancel the dial context — per net.Dialer semantics, the connection must survive.
	dialCancel()
	t.Log("dial context cancelled; connection should survive")

	// Perform an HTTP request "by hand" over the raw connection.
	_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr)
	if err != nil {
		t.Fatalf("write failed after dial context expired: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read failed after dial context expired: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status code after dial context expired: %d", resp.StatusCode)
	}
	t.Log("HTTP request over raw conn succeeded after dial context expired — connection survived")
}
