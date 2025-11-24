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
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
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
	var ctx = context.Background()

	// Initialize client configuration from kubeconfig or in-cluster config
	clientConfig := k8s.GetClientConfig()

	// Extract the REST config and create a clientset for API operations
	rc, err := clientConfig.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	cliSet, err := kubernetes.NewForConfig(rc)
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
	testingNS, _, err := clientConfig.Namespace()
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
	dialer := k8s.NewLazyInitInClusterDialer(clientConfig)
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
	var ctx = context.Background()

	dialer, err := k8s.NewInClusterDialer(ctx, k8s.GetClientConfig())
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
