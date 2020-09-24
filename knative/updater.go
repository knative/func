package knative

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"sort"
	"strings"
	"time"

	. "k8s.io/api/core/v1"
	apiMachineryV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingV1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
)

type Updater struct {
	Verbose   bool
	namespace string
	client    *servingV1client.ServingV1Client
}

func NewUpdater(namespaceOverride string) (updater *Updater, err error) {
	updater = &Updater{}
	config, namespace, err := newClientConfig(namespaceOverride)
	if err != nil {
		return
	}
	updater.namespace = namespace
	updater.client, err = servingV1client.NewForConfig(config)
	return
}

func (updater *Updater) Update(f faas.Function) error {
	client, namespace := updater.client, updater.namespace

	project, err := k8s.ToK8sAllowedName(f.Name)
	if err != nil {
		return fmt.Errorf("updater call to k8s.ToK8sAllowedName failed: %v", err)
	}

	service, err := client.Services(namespace).Get(project, apiMachineryV1.GetOptions{})
	if err != nil {
		return fmt.Errorf("updater failed to get the service: %v", err)
	}

	if service.Spec.Template.Spec.Containers == nil || len(service.Spec.Template.Spec.Containers) < 1 {
		return fmt.Errorf("updater failed to find the container for the service")
	}

	container := &service.Spec.Template.Spec.Containers[0]
	updateBuiltTimeStampEnvVar(container)

	service.Spec.Template.Name, err = generateRevisionName("{{.Service}}-{{.Random 5}}-{{.Generation}}", service)
	if err != nil {
		return fmt.Errorf("updater failed to generate revision name: %v", err)
	}

	_, err = client.Services(namespace).Update(service)
	if err != nil {
		return fmt.Errorf("updater failed to update the service: %v", err)
	}

	return nil
}

func updateBuiltTimeStampEnvVar(container *Container) {
	builtEnvVarName := "BUILT"
	envs := container.Env

	builtEnvVar := findEnvVar(builtEnvVarName, envs)
	if builtEnvVar == nil {
		envs = append(envs, EnvVar{Name: builtEnvVarName})
		builtEnvVar = &envs[len(envs)-1]
	}

	builtEnvVar.Value = time.Now().Format("20060102T150405")

	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name <= envs[j].Name
	})
	container.Env = envs
}

func findEnvVar(name string, envs []EnvVar) *EnvVar {
	var result *EnvVar = nil
	for i, envVar := range envs {
		if envVar.Name == name {
			result = &envs[i]
			break
		}
	}
	return result
}

var charChoices = []string{
	"b", "c", "d", "f", "g", "h", "j", "k", "l", "m", "n", "p", "q", "r", "s", "t", "v", "w", "x",
	"y", "z",
}

type revisionTemplContext struct {
	Service    string
	Generation int64
}

func (c *revisionTemplContext) Random(l int) string {
	chars := make([]string, 0, l)
	for i := 0; i < l; i++ {
		chars = append(chars, charChoices[rand.Int()%len(charChoices)])
	}
	return strings.Join(chars, "")
}

func generateRevisionName(nameTempl string, service *servingv1.Service) (string, error) {
	templ, err := template.New("revisionName").Parse(nameTempl)
	if err != nil {
		return "", err
	}
	context := &revisionTemplContext{
		Service:    service.Name,
		Generation: service.Generation + 1,
	}
	buf := new(bytes.Buffer)
	err = templ.Execute(buf, context)
	if err != nil {
		return "", err
	}
	res := buf.String()
	// Empty is ok.
	if res == "" {
		return res, nil
	}
	prefix := service.Name + "-"
	if !strings.HasPrefix(res, prefix) {
		res = prefix + res
	}
	return res, nil
}
