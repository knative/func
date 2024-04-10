package f

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type F struct {
	Created time.Time
}

func New() *F {
	return &F{time.Now()}
}

func (f *F) Handle(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Request received")
	fmt.Fprintf(w, "Request received\n")
}

func (f *F) Ready(ctx context.Context) (bool, error) {
	// Emulate a function which takes a few seconds to start up.
	if time.Now().After(f.Created.Add(2 * time.Second)) {
		return true, nil
	}
	return false, errors.New("still starting up")
}
