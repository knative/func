package tekton

import (
	"context"
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
			ctx := config.ToContext(context.Background(), cfg)
			task.SetDefaults(ctx)
			apiErr := task.Validate(ctx)
			if apiErr != nil {
				t.Fatalf("%+v\n", apiErr)
			}
		})
	}
}
