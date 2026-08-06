package labels

const (
	FunctionRuntimeKey = "function.knative.dev/runtime"
	FunctionNameKey    = "function.knative.dev/name"

	// FunctionNamespaceKey records the namespace the function itself lives
	// in. It is redundant on an object sitting beside the function, and it is
	// the only thing that tells two functions apart on an object that does
	// not: keda's Routes all share the interceptor's namespace, so a function
	// called "x" in two namespaces yields two Routes there whose name label
	// is identical. Name plus namespace selects exactly one cluster-wide.
	FunctionNamespaceKey = "function.knative.dev/namespace"

	// FunctionKey marks an object as created by func at all. Long-standing;
	// named here so a selector can be built without a literal.
	FunctionKey = "boson.dev/function"
)
