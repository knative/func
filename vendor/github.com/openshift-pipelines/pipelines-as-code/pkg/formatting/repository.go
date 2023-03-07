package formatting

import (
	"github.com/jonboulle/clockwork"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/cli"
)

var shortShaLength = 7

func ShowLastSHA(repository v1alpha1.Repository) string {
	if len(repository.Status) == 0 {
		return nonAttributedStr
	}
	return ShortSHA(*repository.Status[len(repository.Status)-1].SHA)
}

func ShowStatus(repository v1alpha1.Repository, cs *cli.ColorScheme) string {
	if len(repository.Status) == 0 {
		return cs.ColorStatus("NoRun")
	}
	status := repository.Status[len(repository.Status)-1].Status.Conditions[0].GetReason()
	logurl := repository.Status[len(repository.Status)-1].LogURL
	return cs.HyperLink(cs.ColorStatus(status), *logurl)
}

func ShowLastAge(repository v1alpha1.Repository, cw clockwork.Clock) string {
	if len(repository.Status) == 0 {
		return nonAttributedStr
	}
	return Age(repository.Status[len(repository.Status)-1].CompletionTime, cw)
}
