package common

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func WaitForFunctionReady(t *testing.T, functionName string) (revisionName string, functionUrl string) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		revisionName, functionUrl = GetKnativeServiceRevisionAndUrl(t, functionName)
		t.Logf("Waiting function to get ready (revision [%v])", revisionName)
		return revisionName != "", nil
	})
	if err != nil {
		t.Fatal("Function never got ready")
	}
	return revisionName, functionUrl
}

// NewRevisionCheck waits for a new revision to report as ready
func WaitForNewRevisionReady(t *testing.T, previousRevision string, functionName string) (newRevision string) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		newRevision = GetCurrentServiceRevision(t, functionName)
		t.Logf("Waiting for new revision deployment (previous revision [%v], current revision [%v])", previousRevision, newRevision)
		return newRevision != "" && newRevision != previousRevision, nil
	})
	if err != nil {
		t.Fatal("Function new revision never got ready")
	}
	return newRevision
}
