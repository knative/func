package knative

import (
	"fmt"
	"github.com/boson-project/faas/k8s"
	apiCoreV1 "k8s.io/api/core/v1"
	apiMachineryV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	servingV1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	"sort"
	"time"
)

type Updater struct {
	Verbose   bool
	namespace string
	client    *servingV1client.ServingV1Client
}

func NewUpdater(namespace string) (updater *Updater, err error){
	updater = &Updater{}
	updater.namespace = namespace
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return
	}
	updater.client, err = servingV1client.NewForConfig(config)
	if err != nil {
		return
	}
	return
}


func (updater *Updater) Update(name, image string) error {
	client, namespace := updater.client,  updater.namespace

	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return fmt.Errorf("updater call to k8s.ToSubdomain failed: %v", err)
	}

	service, err := client.Services(namespace).Get(project, apiMachineryV1.GetOptions{})
	if err != nil {
		return fmt.Errorf("updater failed to get the service: %v", err)
	}

	if service.Spec.Template.Spec.Containers == nil || len(service.Spec.Template.Spec.Containers) < 1 {
		return fmt.Errorf("updater failed to find the container for the service")
	}

	container := &service.Spec.Template.Spec.Containers[0]

	builtEnvVarName := "BUILT"
	var builtEnvVar *apiCoreV1.EnvVar = nil
	envs := container.Env
	for i, envVar := range envs {
		if envVar.Name == builtEnvVarName {
			builtEnvVar = &envs[i]
			break
		}
	}
	if builtEnvVar == nil {
		envs = append(envs, apiCoreV1.EnvVar{Name: builtEnvVarName})
		builtEnvVar = &envs[len(envs)-1]
	}
	builtEnvVar.Value = time.Now().Format("20060102T150405")

	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name <= envs[j].Name
	})
	container.Env = envs


	_, err = client.Services(namespace).Update(service)
	if err != nil {
		return fmt.Errorf("updater failed to update the service: %v", err)
	}

	return nil
}