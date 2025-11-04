package knative

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	"knative.dev/client/pkg/flags"
	servingclientlib "knative.dev/client/pkg/serving"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/client/pkg/wait"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	KnativeDeployerName = "knative"
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	// verbose logging enablement flag.
	verbose bool

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

// Checks the status of the "user-container" for the ImagePullBackOff reason meaning that
// the container image is not reachable probably because a private registry is being used.
func (d *Deployer) isImageInPrivateRegistry(ctx context.Context, client clientservingv1.KnServingClient, f fn.Function) bool {
	ksvc, err := client.GetService(ctx, f.Name)
	if err != nil {
		return false
	}
	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		return false
	}
	list, err := k8sClient.CoreV1().Pods(f.Deploy.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "serving.knative.dev/revision=" + ksvc.Status.LatestCreatedRevisionName + ",serving.knative.dev/service=" + f.Name,
		FieldSelector: "status.phase=Pending",
	})
	if err != nil {
		return false
	}
	if len(list.Items) != 1 {
		return false
	}

	for _, cont := range list.Items[0].Status.ContainerStatuses {
		if cont.Name == "user-container" {
			return cont.State.Waiting != nil && cont.State.Waiting.Reason == "ImagePullBackOff"
		}
	}
	return false
}

func onClusterFix(f fn.Function) fn.Function {
	// This only exists because of a bootstapping problem with On-Cluster
	// builds:  It appears that, when sending a function to be built on-cluster
	// the target namespace is not being transmitted in the pipeline
	// configuration.  We should figure out how to transmit this information
	// to the pipeline run for initial builds.  This is a new problem because
	// earlier versions of this logic relied entirely on the current
	// kubernetes context.
	if f.Namespace == "" && f.Deploy.Namespace == "" {
		f.Namespace, _ = k8s.GetDefaultNamespace()
	}
	return f
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	f = onClusterFix(f)
	// Choosing f.Namespace vs f.Deploy.Namespace:
	// This is minimal logic currently required of all deployer impls.
	// If f.Namespace is defined, this is the (possibly new) target
	// namespace.  Otherwise use the last deployed namespace.  Error if
	// neither are set.  The logic which arbitrates between curret k8s context,
	// flags, environment variables and global defaults to determine the
	// effective namespace is not logic for the deployer implementation, which
	// should have a minimum of logic.  In this case limited to "new ns or
	// existing namespace?
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
	// TODO: test/consider the case where it HAS been deployed, and the
	// build image has been updated /since/ deployment:  do we need a
	// timestamp? Incrementation?
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Build.Image
	}

	// Clients
	client, err := NewServingClient(namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}
	eventingClient, err := NewEventingClient(namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}
	// check if 'dapr-system' namespace exists
	daprInstalled := false
	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, err
	}
	_, err = k8sClient.CoreV1().Namespaces().Get(ctx, "dapr-system", metav1.GetOptions{})
	if err == nil {
		daprInstalled = true
	}

	var outBuff k8s.SynchronizedBuffer
	var out io.Writer = &outBuff

	if d.verbose {
		out = os.Stderr
	}
	since := time.Now()
	go func() {
		_ = GetKServiceLogs(ctx, namespace, f.Name, f.Deploy.Image, &since, out)
	}()

	previousService, err := client.GetService(ctx, f.Name)
	if err != nil {
		if errors.IsNotFound(err) {

			referencedSecrets := sets.New[string]()
			referencedConfigMaps := sets.New[string]()
			referencedPVCs := sets.New[string]()

			service, err := generateNewService(f, d.decorator, daprInstalled)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = k8s.CheckResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = client.CreateService(ctx, service)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to deploy the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			if d.verbose {
				fmt.Println("Waiting for Knative Service to become ready")
			}
			chprivate := make(chan bool)
			cherr := make(chan error)
			go func() {
				private := false
				for !private {
					time.Sleep(5 * time.Second)
					private = d.isImageInPrivateRegistry(ctx, client, f)
					chprivate <- private
				}
				close(chprivate)
			}()
			go func() {
				err, _ := client.WaitForService(ctx, f.Name,
					clientservingv1.WaitConfig{Timeout: DefaultWaitingTimeout, ErrorWindow: DefaultErrorWindowTimeout},
					wait.NoopMessageCallback())
				cherr <- err
				close(cherr)
			}()

			presumePrivate := false
		main:
			// Wait for either a timeout or a container condition signaling the image is unreachable
			for {
				select {
				case private := <-chprivate:
					if private {
						presumePrivate = true
						break main
					}
				case err = <-cherr:
					break main
				}
			}
			if presumePrivate {
				err := fmt.Errorf("your function image is unreachable. It is possible that your docker registry is private. If so, make sure you have set up pull secrets https://knative.dev/docs/developer/serving/deploying-from-private-registry")
				return fn.DeploymentResult{}, err
			}
			if err != nil {
				err = fmt.Errorf("knative deployer failed to wait for the Knative Service to become ready: %v", err)
				if !d.verbose {
					fmt.Fprintln(os.Stderr, "\nService output:")
					_, _ = io.Copy(os.Stderr, &outBuff)
					fmt.Fprintln(os.Stderr)
				}
				return fn.DeploymentResult{}, err
			}

			route, err := client.GetRoute(ctx, f.Name)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = createTriggers(ctx, f, client, eventingClient)
			if err != nil {
				return fn.DeploymentResult{}, err
			}

			if d.verbose {
				fmt.Printf("Function deployed in namespace %q and exposed at URL:\n%s\n", namespace, route.Status.URL.String())
			}
			return fn.DeploymentResult{
				Status:    fn.Deployed,
				URL:       route.Status.URL.String(),
				Namespace: namespace,
			}, nil

		} else {
			err = fmt.Errorf("knative deployer failed to get the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}
	} else {
		// Update the existing Service
		referencedSecrets := sets.New[string]()
		referencedConfigMaps := sets.New[string]()
		referencedPVCs := sets.New[string]()

		newEnv, newEnvFrom, err := k8s.ProcessEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		newVolumes, newVolumeMounts, err := k8s.ProcessVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		err = k8s.CheckResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		_, err = client.UpdateServiceWithRetry(ctx, f.Name, updateService(f, previousService, newEnv, newEnvFrom, newVolumes, newVolumeMounts, d.decorator, daprInstalled), 3)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		err, _ = client.WaitForService(ctx, f.Name,
			clientservingv1.WaitConfig{Timeout: DefaultWaitingTimeout, ErrorWindow: DefaultErrorWindowTimeout},
			wait.NoopMessageCallback())
		if err != nil {
			if !d.verbose {
				fmt.Fprintln(os.Stderr, "\nService output:")
				_, _ = io.Copy(os.Stderr, &outBuff)
				fmt.Fprintln(os.Stderr)
			}
			return fn.DeploymentResult{}, err
		}

		route, err := client.GetRoute(ctx, f.Name)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
			return fn.DeploymentResult{}, err
		}

		err = createTriggers(ctx, f, client, eventingClient)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		return fn.DeploymentResult{
			Status:    fn.Updated,
			URL:       route.Status.URL.String(),
			Namespace: namespace,
		}, nil
	}
}

func createTriggers(ctx context.Context, f fn.Function, client clientservingv1.KnServingClient, eventingClient clienteventingv1.KnEventingClient) error {
	ksvc, err := client.GetService(ctx, f.Name)
	if err != nil {
		err = fmt.Errorf("knative deployer failed to get the Service for Trigger: %v", err)
		return err
	}

	fmt.Fprintf(os.Stderr, "🎯 Creating Triggers on the cluster\n")

	for i, sub := range f.Deploy.Subscriptions {
		// create the filter:
		attributes := make(map[string]string)
		for key, value := range sub.Filters {
			attributes[key] = value
		}

		err := eventingClient.CreateTrigger(ctx, &eventingv1.Trigger{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-function-trigger-%d", ksvc.GetName(), i),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: ksvc.GroupVersionKind().Version,
						Kind:       ksvc.GroupVersionKind().Kind,
						Name:       ksvc.GetName(),
						UID:        ksvc.GetUID(),
					},
				},
			},
			Spec: eventingv1.TriggerSpec{
				Broker: sub.Source,

				Subscriber: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: ksvc.GroupVersionKind().Version,
						Kind:       ksvc.GroupVersionKind().Kind,
						Name:       ksvc.GetName(),
					}},

				Filter: &eventingv1.TriggerFilter{
					Attributes: attributes,
				},
			},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			err = fmt.Errorf("knative deployer failed to create the Trigger: %v", err)
			return err
		}
	}
	return nil
}

func generateNewService(f fn.Function, decorator deployer.DeployDecorator, daprInstalled bool) (*servingv1.Service, error) {
	container := corev1.Container{
		Image: f.Deploy.Image,
	}

	k8s.SetSecurityContext(&container)
	k8s.SetHealthEndpoints(f, &container)

	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVC := sets.New[string]()

	newEnv, newEnvFrom, err := k8s.ProcessEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	container.Env = newEnv
	container.EnvFrom = newEnvFrom

	newVolumes, newVolumeMounts, err := k8s.ProcessVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVC)
	if err != nil {
		return nil, err
	}
	container.VolumeMounts = newVolumeMounts

	labels, err := deployer.GenerateCommonLabels(f, decorator)
	if err != nil {
		return nil, err
	}

	annotations := generateServiceAnnotations(f, decorator, nil, daprInstalled)

	// we need to create a separate map for Annotations specified in a Revision,
	// in case we will need to specify autoscaling annotations -> these could be only in a Revision not in a Service
	revisionAnnotations := make(map[string]string)
	for k, v := range annotations {
		revisionAnnotations[k] = v
	}

	service := &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: revisionAnnotations,
					},
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								container,
							},
							ServiceAccountName: f.Deploy.ServiceAccountName,
							Volumes:            newVolumes,
						},
					},
				},
			},
		},
	}

	err = setServiceOptions(&service.Spec.Template, f.Deploy.Options)
	if err != nil {
		return service, err
	}

	return service, nil
}

// generateServiceAnnotations creates a final map of service annotations.
// It uses the common annotation generator and adds Knative-specific annotations.
func generateServiceAnnotations(f fn.Function, d deployer.DeployDecorator, previousService *servingv1.Service, daprInstalled bool) (aa map[string]string) {
	// Start with common annotations (includes Dapr, user annotations, and decorator)
	aa = deployer.GenerateCommonAnnotations(f, d, daprInstalled, f.Deploy.Deployer)

	// Set correct creator if we are updating a function (Knative-specific)
	// This annotation is immutable and must be preserved when updating
	if previousService != nil {
		knativeCreatorAnnotation := "serving.knative.dev/creator"
		if val, ok := previousService.Annotations[knativeCreatorAnnotation]; ok {
			aa[knativeCreatorAnnotation] = val
		}
	}

	return
}

func updateService(f fn.Function, previousService *servingv1.Service, newEnv []corev1.EnvVar, newEnvFrom []corev1.EnvFromSource, newVolumes []corev1.Volume, newVolumeMounts []corev1.VolumeMount, decorator deployer.DeployDecorator, daprInstalled bool) func(service *servingv1.Service) (*servingv1.Service, error) {
	return func(service *servingv1.Service) (*servingv1.Service, error) {
		// Removing the name so the k8s server can fill it in with generated name,
		// this prevents conflicts in Revision name when updating the KService from multiple places.
		service.Spec.Template.Name = ""

		annotations := generateServiceAnnotations(f, decorator, previousService, daprInstalled)

		// we need to create a separate map for Annotations specified in a Revision,
		// in case we will need to specify autoscaling annotations -> these could be only in a Revision not in a Service
		revisionAnnotations := make(map[string]string)
		for k, v := range annotations {
			revisionAnnotations[k] = v
		}

		service.Annotations = annotations
		service.Spec.Template.Annotations = revisionAnnotations

		// I hate that we have to do this. Users should not see these values.
		// It is an implementation detail. These health endpoints should not be
		// a part of func.yaml since the user can only mess things up by changing
		// them. Ultimately, this information is determined by the language pack.
		// Which is another reason to consider having a global config to store
		// some metadata which is fairly static. For example, a .config/func/global.yaml
		// file could contain information about all known language packs. As new
		// language packs are discovered through use of the --repository flag when
		// creating a function, this information could be extracted from
		// language-pack.yaml for each template and written to the local global
		// config. At runtime this configuration file could be consulted. I don't
		// know what this would mean for developers using the func library directly.
		cp := &service.Spec.Template.Spec.Containers[0]
		k8s.SetHealthEndpoints(f, cp)

		err := setServiceOptions(&service.Spec.Template, f.Deploy.Options)
		if err != nil {
			return service, err
		}

		labels, err := deployer.GenerateCommonLabels(f, decorator)
		if err != nil {
			return nil, err
		}

		service.Labels = labels
		service.Spec.Template.Labels = labels

		err = flags.UpdateImage(&service.Spec.Template.Spec.PodSpec, f.Deploy.Image)
		if err != nil {
			return service, err
		}

		cp.Env = newEnv
		cp.EnvFrom = newEnvFrom
		cp.VolumeMounts = newVolumeMounts
		service.Spec.Template.Spec.Volumes = newVolumes
		service.Spec.Template.Spec.ServiceAccountName = f.Deploy.ServiceAccountName
		return service, nil
	}
}

// setServiceOptions sets annotations on Service Revision Template or in the Service Spec
// from values specified in function configuration options
func setServiceOptions(template *servingv1.RevisionTemplateSpec, options fn.Options) error {
	toRemove := []string{}
	toUpdate := map[string]string{}

	if options.Scale != nil {
		if options.Scale.Min != nil {
			toUpdate[autoscaling.MinScaleAnnotationKey] = fmt.Sprintf("%d", *options.Scale.Min)
		} else {
			toRemove = append(toRemove, autoscaling.MinScaleAnnotationKey)
		}

		if options.Scale.Max != nil {
			toUpdate[autoscaling.MaxScaleAnnotationKey] = fmt.Sprintf("%d", *options.Scale.Max)
		} else {
			toRemove = append(toRemove, autoscaling.MaxScaleAnnotationKey)
		}

		if options.Scale.Metric != nil {
			toUpdate[autoscaling.MetricAnnotationKey] = *options.Scale.Metric
		} else {
			toRemove = append(toRemove, autoscaling.MetricAnnotationKey)
		}

		if options.Scale.Target != nil {
			toUpdate[autoscaling.TargetAnnotationKey] = fmt.Sprintf("%f", *options.Scale.Target)
		} else {
			toRemove = append(toRemove, autoscaling.TargetAnnotationKey)
		}

		if options.Scale.Utilization != nil {
			toUpdate[autoscaling.TargetUtilizationPercentageKey] = fmt.Sprintf("%f", *options.Scale.Utilization)
		} else {
			toRemove = append(toRemove, autoscaling.TargetUtilizationPercentageKey)
		}

	}

	// in the container always set Requests/Limits & Concurrency values based on the contents of config
	template.Spec.Containers[0].Resources.Requests = nil
	template.Spec.Containers[0].Resources.Limits = nil
	template.Spec.ContainerConcurrency = nil

	if options.Resources != nil {
		if options.Resources.Requests != nil {
			template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{}

			if options.Resources.Requests.CPU != nil {
				value, err := resource.ParseQuantity(*options.Resources.Requests.CPU)
				if err != nil {
					return err
				}
				template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = value
			}

			if options.Resources.Requests.Memory != nil {
				value, err := resource.ParseQuantity(*options.Resources.Requests.Memory)
				if err != nil {
					return err
				}
				template.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = value
			}
		}

		if options.Resources.Limits != nil {
			template.Spec.Containers[0].Resources.Limits = corev1.ResourceList{}

			if options.Resources.Limits.CPU != nil {
				value, err := resource.ParseQuantity(*options.Resources.Limits.CPU)
				if err != nil {
					return err
				}
				template.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU] = value
			}

			if options.Resources.Limits.Memory != nil {
				value, err := resource.ParseQuantity(*options.Resources.Limits.Memory)
				if err != nil {
					return err
				}
				template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] = value
			}

			if options.Resources.Limits.Concurrency != nil {
				template.Spec.ContainerConcurrency = options.Resources.Limits.Concurrency
			}
		}
	}

	return servingclientlib.UpdateRevisionTemplateAnnotations(template, toUpdate, toRemove)
}

func UsesKnativeDeployer(annotations map[string]string) bool {
	deployer, ok := annotations[deployer.DeployerNameAnnotation]

	// annotation is not set (which defines for backwards compatibility the knative deployer)
	// or the deployer is set explicitly to the knative deployer
	return !ok || deployer == KnativeDeployerName
}
