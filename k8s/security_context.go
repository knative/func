package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

func defaultSecurityContext() *corev1.SecurityContext {
	runAsNonRoot := true
	return &corev1.SecurityContext{
		Privileged:               new(bool),
		AllowPrivilegeEscalation: new(bool),
		RunAsNonRoot:             &runAsNonRoot,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           nil,
	}
}
