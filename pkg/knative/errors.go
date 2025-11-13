package knative

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
)

// IsCRDNotFoundError checks if the given error indicates that a requested Kind could not be found and thus the CRD
// most likely is not installed
func IsCRDNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return meta.IsNoMatchError(err) ||
		strings.Contains(err.Error(), "no matches for kind") ||
		strings.Contains(err.Error(), "the server could not find the requested resource") ||
		(
		// check if it's a "knclient.NewInvalidCRD(...)" error)
		strings.HasPrefix(err.Error(), "no or newer Knative ") &&
			strings.HasSuffix(err.Error(), " API found on the backend, please verify the installation or update the 'kn' client"))
}
