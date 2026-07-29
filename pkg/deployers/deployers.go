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

// ValidateSwitch reports an error if an already-deployed function is being
// redeployed with a different deployer. Any change of deployer is refused
// because switching is not supported: nothing reconciles one deployer's
// resources into another's, so the user has to run func delete first and then
// redeploy. Same-deployer redeploys are allowed.
// 'from' is the deployer the function is currently deployed with. 'to' is the
// one to deploy to. An empty value on either side means "not known" -> returns nil.
func ValidateSwitch(from, to string) error {
	if from == "" || to == "" {
		return nil
	}
	if from == to {
		return nil
	}
	return fmt.Errorf("function was deployed with the %q deployer; redeploying with %q is not supported. Run func delete first, then redeploy", from, to)
}
