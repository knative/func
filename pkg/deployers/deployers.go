/*
Package deployers provides canonical short names for the function
deployer implementations.
*/
package deployers

import "fmt"

const (
	Knative    = "knative"
	Kubernetes = "raw"
	Keda       = "keda"

	// Default deployer absent any other configuration.
	Default = Knative
)

// ValidateSwitch reports an error if redeploying an already-deployed function
// with deployer 'to' would strand the previous deployer's resources on the
// cluster. The only safe cross-deployer change is raw -> keda, because the keda
// deployer embeds the raw one; same-deployer redeploys are always allowed.
// 'from' is the deployer the function is currently deployed with. 'to' is the
// one to deploy to. An empty value on either side means "not known" -> returns nil.
func ValidateSwitch(from, to string) error {
	if from == "" || to == "" {
		return nil
	}
	if from == to || (from == Kubernetes && to == Keda) {
		return nil
	}
	return fmt.Errorf("function was deployed with the %q deployer; redeploying with %q would orphan the old deployer's resources on the cluster - run func delete first to remove them, then redeploy", from, to)
}
