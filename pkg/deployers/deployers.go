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
// redeployed with a different deployer. 'from' is the already deployed deployer
// and 'to' is the new deployer "to deploy with"
func ValidateSwitch(from, to string) error {
	if from == "" || to == "" {
		return nil
	}
	if from == to {
		return nil
	}
	return fmt.Errorf("function was deployed with the %q deployer; redeploying with %q is not supported. Run func delete first, then redeploy", from, to)
}
