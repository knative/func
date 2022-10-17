package k8s

import (
	"github.com/Masterminds/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

var oneTwentyFour = semver.MustParse("1.24")

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
