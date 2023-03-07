package formatting

import (
	"github.com/hako/durafmt"
	"github.com/jonboulle/clockwork"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Age(t *metav1.Time, c clockwork.Clock) string {
	if t.IsZero() {
		return nonAttributedStr
	}

	dur := c.Since(t.Time)
	return durafmt.ParseShort(dur).String() + " ago"
}

func Duration(t1, t2 *metav1.Time) string {
	if t1.IsZero() || t2.IsZero() {
		return nonAttributedStr
	}

	dur := t2.Sub(t1.Time)
	return durafmt.ParseShort(dur).String()
}

func PRDuration(runStatus v1alpha1.RepositoryRunStatus) string {
	if runStatus.StartTime == nil {
		return nonAttributedStr
	}

	lasttime := runStatus.CompletionTime
	if lasttime == nil {
		if len(runStatus.Conditions) > 0 {
			lasttime = &runStatus.Conditions[0].LastTransitionTime.Inner
		} else {
			return nonAttributedStr
		}
	}

	return Duration(runStatus.StartTime, lasttime)
}

func Timeout(t *metav1.Duration) string {
	if t == nil {
		return nonAttributedStr
	}

	return durafmt.Parse(t.Duration).String()
}
