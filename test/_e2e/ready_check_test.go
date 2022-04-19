package e2e

import (
	"regexp"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// ReadyCheck waits deployed function to report as ready.
func ReadyCheck(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	expr := regexp.MustCompile("\n" + project.FunctionName + " .*True")
	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (done bool, err error) {
		result := knFunc.Exec("list")
		return result.Error == nil && expr.Match([]byte(result.Stdout)), result.Error
	})
	if err != nil {
		t.Error("Function never get ready")
		t.Fatal()
	}
}

// NewRevisionCheck waits for a new revision to report as ready
func NewRevisionCheck(t *testing.T, previousRevision string, project *FunctionTestProject) (newRevision string) {
	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (done bool, err error) {
		newRevision = GetCurrentServiceRevision(t, project)
		t.Logf("Waiting for new revision deployment (previous revision [%v], current revision [%v])", previousRevision, newRevision)
		return newRevision != "" && newRevision != previousRevision, nil
	})
	if err != nil {
		t.Fatal("Function new revision never got ready")
	}
	return newRevision
}
