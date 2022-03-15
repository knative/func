package function

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1 "knative.dev/serving/pkg/apis/serving/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FunctionName = "Functions"
	// FunctionFile is the file used for the serialized form of a Function.
	FunctionCRDFile = "func-resource.yaml"
)

//FunctionResource Kubernetes resource like function configuration
type FunctionResource struct {

	// Root on disk at which to find/create source and config files. This is not persisted. I don't like this here
	Root string

	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec definition of the function runtime information

	Spec FunctionSpec `json:"spec"`
}

// FunctionSpec
type FunctionSpec struct {
	// ConfigurationSpec holds the latest configuration for the Function PodSpec Template.
	// +optional
	v1.ConfigurationSpec `json:",inline"`
	// FunctionBuildSpec holds the latest configuration for the Function build configuration.
	// +optional
	FunctionBuildSpec `json:"build"`
}

// FunctionBuildSpec
type FunctionBuildSpec struct {
	// BuildType represents the specified way of building the fuction
	// ie. "local" or "git"
	BuildType string `json:"build,omitempty"`

	// Git stores information about remote git repository,
	// in case build type "git" is being used
	Git Git `json:"git,omitempty"`

	// Builder represents the CNCF Buildpack builder image for a function
	Builder string `json:"builder,omitempty"`

	// Build Env variables to be set
	BuildEnvs []Env `json:"buildEnvs,omitempty"`
}

var SchemeGroupVersion = schema.GroupVersion{Group: "func.knative.dev", Version: "v1alpha1"}

// NewFunctionCRD creates a default func-crd yaml file
func NewFunctionCRD(name string) *FunctionResource {
	action := corev1.HTTPGetAction{}
	probe := corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &action,
		},
	}

	return &FunctionResource{
		TypeMeta: metav1.TypeMeta{
			Kind:       FunctionName,
			APIVersion: SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: make(map[string]string),
		},
		Spec: FunctionSpec{
			ConfigurationSpec: v1.ConfigurationSpec{
				Template: v1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: v1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Volumes: []corev1.Volume{},
							Containers: []corev1.Container{
								{
									Name:           name,
									LivenessProbe:  &probe,
									ReadinessProbe: &probe,
								}},
						},
					},
				},
			},
			FunctionBuildSpec: FunctionBuildSpec{
				Git:       Git{},
				BuildEnvs: []Env{},
			},
		},
	}
}

// NewFunctionCRDWith defaults as provided.
func NewFunctionCRDWith(defaults Function) Function {
	if defaults.Version == "" {
		defaults.Version = DefaultVersion
	}
	if defaults.Template == "" {
		defaults.Template = DefaultTemplate
	}
	if defaults.BuildType == "" {
		defaults.BuildType = DefaultBuildType
	}
	return defaults
}

// ValidateCRD Function is logically correct, returning a bundled, and quite
// verbose, formatted error detailing any issues.
func (f FunctionResource) ValidateCRD() error {
	if f.ObjectMeta.Name == "" {
		return errors.New("function name is required")
	}

	var ctr int
	errs := [][]string{
		//validateVolumes(f.Spec.ConfigurationSpec.Template.Spec.PodSpec.Volumes),
		//ValidateEnvs(f.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env),
		//validateOptions(f.Spec.Options),
		//ValidateLabels(f.ObjectMeta.Labels),
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("'%v' contains errors:", FunctionCRDFile))

	for _, ee := range errs {
		if len(ee) > 0 {
			b.WriteString("\n") // Precede each group of errors with a linebreak
		}
		for _, e := range ee {
			ctr++
			b.WriteString("\t" + e)
		}
	}

	if ctr == 0 {
		return nil // Return nil if there were no validation errors.
	}

	return errors.New(b.String())
}

// Write aka (save, serialize, marshal) the Function to disk at its path.
func (f FunctionResource) Write() (err error) {
	path := filepath.Join(f.Root, FunctionCRDFile)
	var bb []byte
	if bb, err = yaml.Marshal(&f); err != nil {
		return
	}
	// TODO: open existing file for writing, such that existing permissions
	// are preserved.
	return ioutil.WriteFile(path, bb, 0644)
}
