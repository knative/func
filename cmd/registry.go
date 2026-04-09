package cmd

import (
	fn "knative.dev/func/pkg/functions"

	"knative.dev/func/pkg/buildpacks"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/s2i"
	"knative.dev/func/pkg/wasm"
)

// newRegistry creates a builder/deployer registry populated with all concrete
// implementations.  Callers receive their own registry instance; there is no
// shared global state.
func newRegistry() *fn.Registry {
	r := fn.NewRegistry()

	// Builders
	buildpacks.Register(r)
	s2i.Register(r)
	oci.Register(r)

	// Deployers
	knative.Register(r)
	k8s.Register(r)
	keda.Register(r)

	// WASM: registers its own builder/deployer AND installs post-processors
	// that restrict traditional builders/deployers from accepting WASI runtimes.
	wasm.Register(r)

	return r
}
