//go:build debug
// +build debug

package runtime

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
)

func recoverMiddleware(handler http.Handler) http.Handler {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	f := func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				recoverError := fmt.Errorf("user function error: %v", r)
				stack := string(debug.Stack())
				logger.Printf("%v\n%v\n", recoverError, stack)

				rw.WriteHeader(http.StatusInternalServerError)

				rw.Header().Add("Content-Type", "text/plain")
				fmt.Fprintln(rw, recoverError)
				fmt.Fprintln(rw, stack)
			}
		}()
		handler.ServeHTTP(rw, req)
		return
	}
	return http.HandlerFunc(f)
}
