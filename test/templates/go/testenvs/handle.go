package function

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")
	testEnvVars := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "TEST_") {
			testEnvVars = append(testEnvVars, e)
		}
	}
	fmt.Fprintf(w, "%v\n", strings.Join(testEnvVars, "\n"))
}
