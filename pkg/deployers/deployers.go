/*
Package deployers provides constants for deployer implementation short names.
*/
package deployers

const (
	Knative = "knative"
	Raw     = "raw"
	Keda    = "keda"
	Wasm    = "wasm"
)

// All returns all known deployer names
func All() []string {
	return []string{Knative, Raw, Keda, Wasm}
}
