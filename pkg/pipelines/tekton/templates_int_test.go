//go:build integration

package tekton

import (
	"testing"

	"github.com/manifestival/manifestival"
	"github.com/manifestival/manifestival/fake"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

func Test_createAndApplyPipelineTemplate(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			// save current function and restore it at the end
			old := manifestivalClient
			defer func() { manifestivalClient = old }()

			manifestivalClient = func() (manifestival.Client, error) {
				return fake.New(), nil
			}

			root := tt.root
			defer Using(t, root)()

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}

			f.Build.Builder = tt.builder
			f.Runtime = tt.runtime
			f.Image = "docker.io/alice/" + f.Name
			f.Registry = TestRegistry

			if err := createAndApplyPipelineTemplate(f, tt.namespace, tt.labels); (err != nil) != tt.wantErr {
				t.Errorf("createAndApplyPipelineTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
