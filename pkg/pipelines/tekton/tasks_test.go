package tekton

import (
	"testing"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// Smoke tests.
// We check that we can get a task (without a panic) and that the task is a valid tekton task.
func TestGetTasks(t *testing.T) {

	tests := []struct {
		name    string
		getTask func() string
	}{
		{
			name:    "s2i",
			getTask: getS2ITask,
		},
		{
			name:    "pack",
			getTask: getBuildpackTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myScheme := runtime.NewScheme()
			if err := tektonv1.AddToScheme(myScheme); err != nil {
				t.Fatal(err)
			}
			codecs := serializer.NewCodecFactory(myScheme)
			decode := codecs.UniversalDeserializer().Decode
			obj, _, err := decode([]byte(tt.getTask()), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			task, ok := obj.(*tektonv1.Task)
			if !ok {
				t.Fatalf("unexpected type: %T", obj)
			}
			t.Logf("successfully decoded task: %s\n", task.Name)

			// Run deeper validations on the task
			flags, err := config.NewFeatureFlagsFromMap(map[string]string{
				"enable-api-fields": "alpha",
			})
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{
				FeatureFlags: flags,
			}
			ctx := config.ToContext(t.Context(), cfg)
			task.SetDefaults(ctx)
			apiErr := task.Validate(ctx)
			if apiErr != nil {
				t.Fatalf("%+v\n", apiErr)
			}

			// The workspace is emptied as root right before the clone, and
			// only when there is a repository to clone; the upload path must
			// keep the sources it received.
			steps := task.Spec.Steps
			if len(steps) < 2 || steps[0].Name != "clean-src" || steps[1].Name != "fetch-src" {
				t.Fatalf("expected clean-src to precede fetch-src, got %v", stepNames(steps))
			}
			if len(steps[0].When) != 1 || steps[0].When[0].Input != "$(params.GIT_REPOSITORY)" {
				t.Errorf("expected clean-src to be gated on GIT_REPOSITORY, got %+v", steps[0].When)
			}
		})
	}
}

func stepNames(steps []tektonv1.Step) []string {
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		names = append(names, s.Name)
	}
	return names
}
