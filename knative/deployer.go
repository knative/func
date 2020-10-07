package knative

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/client/pkg/wait"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
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
	_, namespace, err := newClientConfig(namespaceOverride)
	if err != nil {
		return
	}
	deployer.Namespace = namespace
	//	deployer.client, err = servingv1client.NewForConfig(config)
	return
}

func (d *Deployer) Deploy(f faas.Function) (err error) {

	// k8s does not support service names with dots. so encode it such that
	// www.my-domain,com -> www-my--domain-com
	encodedName, err := k8s.ToK8sAllowedName(f.Name)
	if err != nil {
		return
	}

	client, output, err := NewClient(d.Namespace, d.Verbose)
	if err != nil {
		return
	}

	_, err = client.GetService(encodedName)
	if err != nil {
		if errors.IsNotFound(err) {

			// Let's create a new Service
			err := client.CreateService(generateNewService(encodedName, f.Image))
			if err != nil {
				if !d.Verbose {
					err = fmt.Errorf("failed to deploy the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
				} else {
					err = fmt.Errorf("failed to deploy the service: %v", err)
				}
				return err
			}

			err, _ = client.WaitForService(encodedName, DefaultWaitingTimeout, wait.NoopMessageCallback())
			if err != nil {
				if !d.Verbose {
					err = fmt.Errorf("deployer failed to wait for the service to become ready: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
				} else {
					err = fmt.Errorf("deployer failed to wait for the service to become ready: %v", err)
				}
				return err
			}

			route, err := client.GetRoute(encodedName)
			if err != nil {
				if !d.Verbose {
					err = fmt.Errorf("deployer failed to get the route: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
				} else {
					err = fmt.Errorf("deployer failed to get the route: %v", err)
				}
				return err
			}

			fmt.Println("Function deployed on: " + route.Status.URL.String())

		} else {
			if !d.Verbose {
				err = fmt.Errorf("deployer failed to get the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
			} else {
				err = fmt.Errorf("deployer failed to get the service: %v", err)
			}
			return err
		}
	} else {
		// Update the existing Service
		err = client.UpdateServiceWithRetry(encodedName, updateBuiltTimeStampEnvVar, 3)
		if err != nil {
			if !d.Verbose {
				err = fmt.Errorf("deployer failed to update the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
			} else {
				err = fmt.Errorf("deployer failed to update the service: %v", err)
			}
			return err
		}
	}

	return nil
}

func generateNewService(name, image string) *servingv1.Service {
	containers := []corev1.Container{
		{
			Image: image,
			Env: []corev1.EnvVar{
				{Name: "VERBOSE", Value: "true"},
			},
		},
	}

	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"bosonFunction": "true",
			},
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
}

func updateBuiltTimeStampEnvVar(service *servingv1.Service) (*servingv1.Service, error) {
	envs := service.Spec.Template.Spec.Containers[0].Env

	builtEnvVarName := "BUILT"

	builtEnvVar := findEnvVar(builtEnvVarName, envs)
	if builtEnvVar == nil {
		envs = append(envs, corev1.EnvVar{Name: "VERBOSE", Value: "true"})
		builtEnvVar = &envs[len(envs)-1]
	}

	builtEnvVar.Value = time.Now().Format("20060102T150405")

	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name <= envs[j].Name
	})
	service.Spec.Template.Spec.Containers[0].Env = envs

	return service, nil
}

func findEnvVar(name string, envs []corev1.EnvVar) *corev1.EnvVar {
	var result *corev1.EnvVar = nil
	for i, envVar := range envs {
		if envVar.Name == name {
			result = &envs[i]
			break
		}
	}
	return result
}
