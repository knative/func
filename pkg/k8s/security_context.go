package k8s

import (
	"github.com/Masterminds/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

var oneTwentyFour = semver.MustParse("1.24")

func defaultPodSecurityContext() *corev1.PodSecurityContext {
	// change ownership of the mounted volume to the first non-root user uid=1000
	if IsOpenShift() {
		return nil
	}
	runAsUser := int64(1001)
	runAsGroup := int64(1002)
	return &corev1.PodSecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
		FSGroup:    &runAsGroup,
	}
}

func defaultSecurityContext(client *kubernetes.Clientset) *corev1.SecurityContext {
	runAsNonRoot := true
	sc := &corev1.SecurityContext{
		Privileged:               new(bool),
		AllowPrivilegeEscalation: new(bool),
		RunAsNonRoot:             &runAsNonRoot,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           nil,
	}
	if info, err := client.ServerVersion(); err == nil {
		var v *semver.Version
		v, err = semver.NewVersion(info.String())
		if err == nil && v.Compare(oneTwentyFour) >= 0 {
			sc.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
		}
	}
	return sc
}
