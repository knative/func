package keda

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
)

func TestDeployer_interceptorBridgeService(t *testing.T) {
	tests := []struct {
		name                 string
		function             fn.Function
		expectedExternalName string
	}{
		{
			name: "uses default cluster.local when ClusterDomain is empty",
			function: fn.Function{
				Name: "test-func",
				Deploy: fn.DeploySpec{
					ClusterDomain: "",
				},
			},
			expectedExternalName: "keda-add-ons-http-interceptor-proxy.keda.svc.cluster.local",
		},
		{
			name: "uses custom cluster domain when specified",
			function: fn.Function{
				Name: "test-func",
				Deploy: fn.DeploySpec{
					ClusterDomain: "custom.domain",
				},
			},
			expectedExternalName: "keda-add-ons-http-interceptor-proxy.keda.svc.custom.domain",
		},
		{
			name: "uses cluster.local explicitly when specified",
			function: fn.Function{
				Name: "test-func",
				Deploy: fn.DeploySpec{
					ClusterDomain: "cluster.local",
				},
			},
			expectedExternalName: "keda-add-ons-http-interceptor-proxy.keda.svc.cluster.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeployer()
			deployment := &v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.function.Name,
					UID:  "test-uid",
				},
			}

			service := d.interceptorBridgeService(tt.function, "test-namespace", deployment)

			if service.Spec.ExternalName != tt.expectedExternalName {
				t.Errorf("interceptorBridgeService() ExternalName = %v, want %v",
					service.Spec.ExternalName, tt.expectedExternalName)
			}

			// Verify service name
			expectedServiceName := d.interceptorBridgeServiceName(tt.function)
			if service.Name != expectedServiceName {
				t.Errorf("interceptorBridgeService() Name = %v, want %v",
					service.Name, expectedServiceName)
			}

			// Verify namespace
			if service.Namespace != "test-namespace" {
				t.Errorf("interceptorBridgeService() Namespace = %v, want %v",
					service.Namespace, "test-namespace")
			}

			// Verify service type
			if service.Spec.Type != "ExternalName" {
				t.Errorf("interceptorBridgeService() Type = %v, want %v",
					service.Spec.Type, "ExternalName")
			}
		})
	}
}
