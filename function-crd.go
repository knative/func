package function

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FunctionName = "Functions"
	// FunctionFile is the file used for the serialized form of a Function.
	FunctionCRDFile = "func-crd.yaml"
)

type FunctionCRD struct {

	// Root on disk at which to find/create source and config files. This is not persisted. I don't like this here
	Root string

	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec definition of the function runtime information
	Spec FunctionSpec `json:"spec"`
}

type FunctionSpec struct {

	// Root on disk at which to find/create source and config files.
	Root string `json:"-"`

	// Optional full OCI image tag in form:
	//   [registry]/[namespace]/[name]:[tag]
	// example:
	//   quay.io/alice/my.function.name
	// Registry is optional and is defaulted to DefaultRegistry
	// example:
	//   alice/my.function.name
	// If Image is provided, it overrides the default of concatenating
	// "Registry+Name:latest" to derive the Image.
	Image string `json:"image"`

	// Map containing user-supplied annotations
	// Example: { "division": "finance" }
	Annotations map[string]string `json:"annotations,omitempty"`

	// Options to be set on deployed function (scaling, etc.)
	Options Options `json:"options,omitempty"`

	// Map of user-supplied labels
	Labels []Label `json:"labels,omitempty"`

	// Health endpoints specified by the language pack
	HealthEndpoints HealthEndpoints `json:"healthEndpoints,omitempty"`

	// Invocation defines hints for use when invoking this function.
	// See Client.Invoke for usage.
	Invocation Invocation `json:"invocation,omitempty"`

	// List of volumes to be mounted to the function
	Volumes []Volume `json:"volumes,omitempty"`

	// Env variables to be set
	Envs []Env `json:"envs,omitempty"`
}

var SchemeGroupVersion = schema.GroupVersion{Group: "func.knative.dev", Version: "v1alpha1"}

// NewFunctionCRD creates a default func-crd yaml file
func NewFunctionCRD() *FunctionCRD {
	return &FunctionCRD{
		TypeMeta: metav1.TypeMeta{
			Kind:       FunctionName,
			APIVersion: SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: make(map[string]string),
		},
		Spec: FunctionSpec{},
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
func (f FunctionCRD) ValidateCRD() error {
	if f.ObjectMeta.Name == "" {
		return errors.New("function name is required")
	}

	var ctr int
	errs := [][]string{
		validateVolumes(f.Spec.Volumes),
		ValidateEnvs(f.Spec.Envs),
		validateOptions(f.Spec.Options),
		ValidateLabels(f.Spec.Labels),
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
func (f FunctionCRD) Write() (err error) {
	path := filepath.Join(f.Root, FunctionCRDFile)
	var bb []byte
	if bb, err = yaml.Marshal(&f); err != nil {
		return
	}
	// TODO: open existing file for writing, such that existing permissions
	// are preserved.
	return ioutil.WriteFile(path, bb, 0644)
}
