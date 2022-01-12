package labels

const (
	FunctionKey        = "function.knative.dev"
	FunctionValue      = "true"
	FunctionRuntimeKey = "function.knative.dev/runtime"
	FunctionNameKey    = "function.knative.dev/name"

	// --- handle usage of deprecated labels
	DeprecatedFunctionKey        = "boson.dev/function"
	DeprecatedFunctionRuntimeKey = "boson.dev/runtime"
	// --- end of handling usage of deprecated runtime labels
)
