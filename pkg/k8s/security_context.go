package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

// defaultPodSecurityContext returns a PodSecurityContext that satisfies the
// Kubernetes "restricted" pod security profile (requires k8s >= 1.25; this
// project tracks k8s client-go v0.35 / k8s 1.35).
//
// SeccompProfile is set at both pod and container level (see defaultSecurityContext)
// as defence-in-depth: pod-level covers all containers by default, container-level
// ensures compliance even if a pod-level context is ever overridden downstream.
//
// RunAsGroup: 0 (root group) is retained on non-OpenShift to preserve compatibility
// with Tekton buildpack tasks that mount volumes with group ownership 0.
// This does not violate the restricted profile (which checks UID, not GID) but is
// tracked for remediation in https://github.com/knative/func/issues/3517.
func defaultPodSecurityContext() *corev1.PodSecurityContext {
	runAsNonRoot := true
	seccompProfile := &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}

	if IsOpenShift() {
		// On OpenShift, SCCs manage RunAsUser/RunAsGroup/FSGroup; setting them
		// here would conflict with the namespace's SCC UID range policy.
		// Only set the fields required by the restricted PSA profile.
		return &corev1.PodSecurityContext{
			RunAsNonRoot:   &runAsNonRoot,
			SeccompProfile: seccompProfile,
		}
	}

	runAsUser := int64(1001)
	runAsGroup := int64(0) // Match Tekton buildpack task group; see doc comment above.
	fsGroup := int64(1002) // Keep FSGroup for volume ownership
	return &corev1.PodSecurityContext{
		RunAsNonRoot:   &runAsNonRoot,
		SeccompProfile: seccompProfile,
		RunAsUser:      &runAsUser,
		RunAsGroup:     &runAsGroup,
		FSGroup:        &fsGroup,
	}
}

// defaultSecurityContext returns a container SecurityContext that satisfies the
// Kubernetes "restricted" pod security profile.
// SeccompProfile is set unconditionally; RuntimeDefault has been GA since k8s 1.25.
func defaultSecurityContext() *corev1.SecurityContext {
	privileged := false
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	return &corev1.SecurityContext{
		Privileged:               &privileged,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		RunAsNonRoot:             &runAsNonRoot,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		// SeccompProfile is also set at pod level; both levels are set intentionally
		// for defence-in-depth (see defaultPodSecurityContext doc comment).
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}
