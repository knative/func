package k8s

import (
	"testing"
)

func TestDetectSocatSuccess(t *testing.T) {
	ch := make(chan struct{}, 1)
	w := detectConnSuccess(ch)
	_, err := w.Write([]byte("some data successucces"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte("sfully connected"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Log("OK")
	default:
		t.Error("NOK")
	}
}
