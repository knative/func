package functions

import (
	"fmt"
)

// ValidateExpose reports whether expose is a valid deploy.expose value: ""
// (default - exposed via an OpenShift Route, since a deployed function
// being reachable is the expected outcome; cluster-local on non-OpenShift
// clusters, since a Route is an OpenShift-only mechanism), "none"
// (cluster-local, explicit opt-out), or "route" (explicit request for an
// OpenShift Route). There is no ref suffix: an OpenShift Route has no
// concept of "which ingress controller to attach to" - the cluster's
// IngressController picks the router, and the Route object doesn't
// reference one. Any other value is rejected.
func ValidateExpose(expose string) error {
	if expose == "" || expose == "none" || expose == "route" {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidExpose, expose)
}
