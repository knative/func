package knative

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/client/pkg/kn/flags"
	"knative.dev/client/pkg/wait"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"

	fn "github.com/boson-project/func"
)

type Deployer struct {
	// Namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, deployment will commence to the configured namespace.
	Namespace string
	// Verbose logging enablement flag.
	Verbose bool
}

func NewDeployer(namespaceOverride string) (deployer *Deployer, err error) {
	deployer = &Deployer{}
	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	deployer.Namespace = namespace
	return
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (result fn.DeploymentResult, err error) {

	client, err := NewServingClient(d.Namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	_, err = client.GetService(f.Name)
	if err != nil {
		if errors.IsNotFound(err) {

			service, err := generateNewService(f.Name, f.ImageWithDigest(), f.Runtime, f.Env, f.Annotations)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the service: %v", err)
				return fn.DeploymentResult{}, err
			}
			err = client.CreateService(service)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to deploy the service: %v", err)
				return fn.DeploymentResult{}, err
			}

			if d.Verbose {
				fmt.Println("Waiting for Knative Service to become ready")
			}
			err, _ = client.WaitForService(f.Name, DefaultWaitingTimeout, wait.NoopMessageCallback())
			if err != nil {
				err = fmt.Errorf("knative deployer failed to wait for the service to become ready: %v", err)
				return fn.DeploymentResult{}, err
			}

			route, err := client.GetRoute(f.Name)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to get the route: %v", err)
				return fn.DeploymentResult{}, err
			}

			fmt.Println("Function deployed at URL: " + route.Status.URL.String())
			return fn.DeploymentResult{
				Status: fn.Deployed,
				URL:    route.Status.URL.String(),
			}, nil
			// fmt.Sprintf("Function deployed at URL: %v", route.Status.URL.String()), nil

		} else {
			err = fmt.Errorf("knative deployer failed to get the service: %v", err)
			return fn.DeploymentResult{}, err
		}
	} else {
		// Update the existing Service
		_, err = client.UpdateServiceWithRetry(f.Name, updateService(f.ImageWithDigest(), f.Env, f.Annotations), 3)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the service: %v", err)
			return fn.DeploymentResult{}, err
		}

		route, err := client.GetRoute(f.Name)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to get the route: %v", err)
			return fn.DeploymentResult{}, err
		}

		return fn.DeploymentResult{
			Status: fn.Updated,
			URL:    route.Status.URL.String(),
		}, nil
	}
}

func probeFor(url string) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: url,
			},
		},
	}
}

func generateNewService(name, image, runtime string, env map[string]string, annotations map[string]string) (*servingv1.Service, error) {
	containers := []corev1.Container{
		{
			Image: image,
		},
	}

	if runtime != "quarkus" {
		containers[0].LivenessProbe = probeFor("/health/liveness")
		containers[0].ReadinessProbe = probeFor("/health/readiness")
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"boson.dev/function": "true",
				"boson.dev/runtime":  runtime,
			},
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			ConfigurationSpec: v1.ConfigurationSpec{
				Template: v1.RevisionTemplateSpec{
					Spec: v1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: containers,
						},
					},
				},
			},
		},
	}

	return setEnv(service, env)
}

func updateService(image string, env map[string]string, annotations map[string]string) func(service *servingv1.Service) (*servingv1.Service, error) {
	return func(service *servingv1.Service) (*servingv1.Service, error) {
		// Removing the name so the k8s server can fill it in with generated name,
		// this prevents conflicts in Revision name when updating the KService from multiple places.
		service.Spec.Template.Name = ""

		// Don't bother being as clever as we are with env variables
		// Just set the annotations to be whatever we find in func.yaml
		for k, v := range annotations {
			service.ObjectMeta.Annotations[k] = v
		}

		err := flags.UpdateImage(&service.Spec.Template.Spec.PodSpec, image)
		if err != nil {
			return service, err
		}
		return setEnv(service, env)
	}
}

var evRegex = regexp.MustCompile(`^{{\s*(\w+)\s*.(\w+)\s*}}$`)

const (
	ctxIdx = 1
	valIdx = 2
)

func processValue(val string) (string, error) {
	match := evRegex.FindStringSubmatch(val)
	if len(match) > valIdx {
		if match[ctxIdx] != "env" {
			return "", fmt.Errorf("uknown context %q", match[ctxIdx])
		}
		if v, ok := os.LookupEnv(match[valIdx]); ok {
			return v, nil
		} else {
			return "", fmt.Errorf("required environment variable %q is not set", match[valIdx])
		}
	} else {
		return val, nil
	}
}

func setEnv(service *servingv1.Service, env map[string]string) (*servingv1.Service, error) {
	builtEnvName := "BUILT"
	builtEnvValue := time.Now().Format("20060102T150405")

	toUpdate := make(map[string]string, len(env)+1)
	toRemove := make([]string, 0)

	for name, value := range env {
		if strings.HasSuffix(name, "-") {
			toRemove = append(toRemove, strings.TrimSuffix(name, "-"))
		} else {
			toUpdate[name] = value
		}
	}

	toUpdate[builtEnvName] = builtEnvValue

	for idx, val := range toUpdate {
		v, err := processValue(val)
		if err != nil {
			return nil, err
		}
		toUpdate[idx] = v
	}

	return service, flags.UpdateEnvVars(&service.Spec.Template.Spec.PodSpec, toUpdate, toRemove)
}
