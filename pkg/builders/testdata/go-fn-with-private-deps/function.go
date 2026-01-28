package function

import (
	"fmt"
	"net/http"

	"git-private.localtest.me/foo.git/pkg/foo"
)

type Function struct{}

func New() *Function { return &Function{} }

// Handle an HTTP Request.
func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")
	w.WriteHeader(200)
	_, _ = fmt.Fprintf(w, "The answer is: %d", foo.Foo())
}
