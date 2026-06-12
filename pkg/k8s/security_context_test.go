package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func testClient(openshift bool) *Client {
	return NewClientWithOpenShift(nil, openshift)
}

func TestDefaultPodSecurityContext_NonOpenShift(t *testing.T) {
	kc := testClient(false)

	sc := defaultPodSecurityContext(kc)
	if sc == nil {
		t.Fatal("expected non-nil PodSecurityContext on non-OpenShift")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true")
	}
	if sc.SeccompProfile == nil {
		t.Fatal("expected SeccompProfile to be set")
	}
	if sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected SeccompProfile.Type=RuntimeDefault, got %v", sc.SeccompProfile.Type)
	}
	if sc.RunAsUser == nil || *sc.RunAsUser == 0 {
		t.Error("expected non-zero RunAsUser on non-OpenShift")
	}
	if sc.FSGroup == nil {
		t.Error("expected FSGroup to be set on non-OpenShift")
	}
}

func TestDefaultPodSecurityContext_OpenShift(t *testing.T) {
	kc := testClient(true)

	sc := defaultPodSecurityContext(kc)
	if sc == nil {
		t.Fatal("expected non-nil PodSecurityContext on OpenShift")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true on OpenShift")
	}
	if sc.SeccompProfile == nil {
		t.Fatal("expected SeccompProfile to be set on OpenShift")
	}
	if sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected SeccompProfile.Type=RuntimeDefault, got %v", sc.SeccompProfile.Type)
	}
	if sc.RunAsUser != nil {
		t.Errorf("expected RunAsUser to be nil on OpenShift, got %d", *sc.RunAsUser)
	}
	if sc.RunAsGroup != nil {
		t.Errorf("expected RunAsGroup to be nil on OpenShift, got %d", *sc.RunAsGroup)
	}
	if sc.FSGroup != nil {
		t.Errorf("expected FSGroup to be nil on OpenShift, got %d", *sc.FSGroup)
	}
}

func TestDefaultSecurityContext(t *testing.T) {
	sc := defaultSecurityContext()
	if sc == nil {
		t.Fatal("expected non-nil SecurityContext")
	}
	if sc.Privileged == nil || *sc.Privileged {
		t.Error("expected Privileged=false (explicit)")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("expected AllowPrivilegeEscalation=false")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true")
	}
	if sc.Capabilities == nil {
		t.Fatal("expected Capabilities to be set")
	}
	if len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Error("expected Capabilities.Drop=[ALL]")
	}
	if sc.SeccompProfile == nil {
		t.Fatal("expected SeccompProfile to be set")
	}
	if sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected SeccompProfile.Type=RuntimeDefault, got %v", sc.SeccompProfile.Type)
	}
}

func TestRestrictedProfileCompliance(t *testing.T) {
	for _, openshift := range []bool{false, true} {
		openshift := openshift
		name := "non-openshift"
		if openshift {
			name = "openshift"
		}
		t.Run(name, func(t *testing.T) {
			kc := testClient(openshift)

			pod := defaultPodSecurityContext(kc)
			ctr := defaultSecurityContext()

			if ctr.AllowPrivilegeEscalation == nil || *ctr.AllowPrivilegeEscalation {
				t.Error("restricted violation: AllowPrivilegeEscalation must be false")
			}
			hasDropAll := false
			if ctr.Capabilities != nil {
				for _, cap := range ctr.Capabilities.Drop {
					if cap == corev1.Capability("ALL") {
						hasDropAll = true
						break
					}
				}
			}
			if !hasDropAll {
				t.Error("restricted violation: capabilities.drop must include ALL")
			}
			if pod.RunAsNonRoot == nil || !*pod.RunAsNonRoot {
				t.Error("restricted violation: runAsNonRoot must be true")
			}
			if pod.SeccompProfile == nil {
				t.Error("restricted violation: seccompProfile must be set at pod level")
			}
			if ctr.SeccompProfile == nil {
				t.Error("restricted violation: seccompProfile must be set at container level")
			}
		})
	}
}
