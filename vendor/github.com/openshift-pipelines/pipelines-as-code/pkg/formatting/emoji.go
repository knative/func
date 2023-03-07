package formatting

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	knative1 "knative.dev/pkg/apis/duck/v1"
)

const nonAttributedStr = "---"

// formatCondition knative formatcondition with emoji or not
func formatCondition(c knative1.Conditions, skipemoji bool) string {
	var status, emoji string
	if len(c) == 0 {
		return nonAttributedStr
	}

	switch c[0].Status {
	case corev1.ConditionFalse:
		emoji = "❌"
		status = "Failed"
	case corev1.ConditionTrue:
		emoji = "✅"
		status = "Succeeded"
	case corev1.ConditionUnknown:
		emoji = "🏃"
		status = "Running"
	}
	if !skipemoji {
		status = fmt.Sprintf("%s %s", emoji, status)
	}

	return status
}

func ConditionEmoji(c knative1.Conditions) string {
	return formatCondition(c, false)
}

func ConditionSad(c knative1.Conditions) string {
	return formatCondition(c, true)
}
