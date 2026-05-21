package http

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"
	"sync"

	"knative.dev/func/pkg/k8s"
)

const openShiftRegistryHost = "image-registry.openshift-image-registry.svc"

// WithOpenShiftServiceCA enables trust to OpenShift's service CA for internal image registry
func WithOpenShiftServiceCA(c *k8s.Client) Option {
	var err error
	var ca *x509.Certificate
	var o sync.Once

	selectCA := func(ctx context.Context, serverName string) (*x509.Certificate, error) {
		if c != nil && strings.HasPrefix(serverName, openShiftRegistryHost) {
			o.Do(func() {
				ca, err = c.GetOpenShiftServiceCA(ctx)
				if err != nil {
					err = fmt.Errorf("cannot get CA: %w", err)
				}
			})
			if err != nil {
				return nil, err
			}
			return ca, nil
		}
		return nil, nil
	}

	return WithSelectCA(selectCA)
}
