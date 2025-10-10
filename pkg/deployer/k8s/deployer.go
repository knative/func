package k8s

import (
	"context"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	verbose   bool
	decorator deployer.DeployDecorator
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
	}
}

func WithDeployerDecorator(decorator deployer.DeployDecorator) DeployerOpt {
	return func(d *Deployer) {
		d.decorator = decorator
	}
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	namespace := f.Namespace
	if namespace == "" {
		namespace = f.Deploy.Namespace
	}
	if namespace == "" {
		return fn.DeploymentResult{}, fmt.Errorf("deployer requires either a target namespace or that the function be already deployed")
	}

	// Choosing an image to deploy:
	// If the service has not been deployed before, but there exists a
	// build image, this build image should be used for the deploy.
	// TODO: test/consdier the case where it HAS been deployed, and the
	// build image has been updated /since/ deployment:  do we need a
	// timestamp? Incrementation?
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Build.Image
	}

	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	// Check if Dapr is installed
	daprInstalled := false
	_, err = clientset.CoreV1().Namespaces().Get(ctx, "dapr-system", metav1.GetOptions{})
	if err == nil {
		daprInstalled = true
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)
	eventingClient, err := knative.NewEventingClient(namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	existingDeployment, err := deploymentClient.Get(ctx, f.Name, metav1.GetOptions{})

	var status fn.Status
	if err == nil {
		deployment, svc, err := d.generateResources(f, namespace, daprInstalled)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate resources: %w", err)
		}

		// Preserve resource version for update
		deployment.ResourceVersion = existingDeployment.ResourceVersion

		if _, err = deploymentClient.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to update deployment: %w", err)
		}

		existingService, err := serviceClient.Get(ctx, f.Name, metav1.GetOptions{})
		if err == nil {
			svc.ResourceVersion = existingService.ResourceVersion
			if _, err = serviceClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to update service: %w", err)
			}
		} else if errors.IsNotFound(err) {
			// Service doesn't exist, create it
			if _, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to create service: %w", err)
			}
		} else {
			return fn.DeploymentResult{}, fmt.Errorf("failed to get existing service: %w", err)
		}

		err = createTriggers(ctx, f, serviceClient, eventingClient)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		status = fn.Updated
		if d.verbose {
			fmt.Fprintf(os.Stderr, "Updated deployment and service %s in namespace %s\n", f.Name, namespace)
		}
	} else {
		if !errors.IsNotFound(err) {
			return fn.DeploymentResult{}, fmt.Errorf("failed to check for existing deployment: %w", err)
		}

		deployment, svc, err := d.generateResources(f, namespace, daprInstalled)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate resources: %w", err)
		}

		if _, err = deploymentClient.Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create deployment: %w", err)
		}

		if _, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create service: %w", err)
		}

		err = createTriggers(ctx, f, serviceClient, eventingClient)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		status = fn.Deployed
		if d.verbose {
			fmt.Fprintf(os.Stderr, "Created deployment and service %s in namespace %s\n", f.Name, namespace)
		}
	}

	url := fmt.Sprintf("http://%s.%s.svc.cluster.local", f.Name, namespace)

	return fn.DeploymentResult{
		Status:    status,
		URL:       url,
		Namespace: namespace,
	}, nil
}

func (d *Deployer) generateResources(f fn.Function, namespace string, daprInstalled bool) (*appsv1.Deployment, *corev1.Service, error) {
	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return nil, nil, err
	}

	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, daprInstalled)

	// Use annotations for pod template
	podAnnotations := make(map[string]string)
	for k, v := range annotations {
		podAnnotations[k] = v
	}

	// Process environment variables and volumes
	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	envVars, envFrom, err := deployer.ProcessEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process environment variables: %w", err)
	}

	volumes, volumeMounts, err := deployer.ProcessVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process volumes: %w", err)
	}

	container := corev1.Container{
		Name:  "user-container",
		Image: f.Deploy.Image,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: deployer.DefaultHTTPPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          envVars,
		EnvFrom:      envFrom,
		VolumeMounts: volumeMounts,
	}

	deployer.SetHealthEndpoints(f, &container)
	deployer.SetSecurityContext(&container)

	replicas := int32(1)
	if f.Deploy.Options.Scale != nil && f.Deploy.Options.Scale.Min != nil && *f.Deploy.Options.Scale.Min > 0 {
		replicas = int32(*f.Deploy.Options.Scale.Min)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					ServiceAccountName: f.Deploy.ServiceAccountName,
					Volumes:            volumes,
				},
			},
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(deployer.DefaultHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return deployment, service, nil
}

func createTriggers(ctx context.Context, f fn.Function, serviceClient v1.ServiceInterface, eventingClient clienteventingv1.KnEventingClient) error {
	svc, err := serviceClient.Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("failed to get the Service for Trigger: %v", err)
		return err
	}

	return deployer.CreateTriggers(ctx, f, svc, eventingClient)
}
