package function

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

type Function struct {
	recorderURL string
}

func New() *Function {
	return &Function{recorderURL: os.Getenv("RECORDER_URL")}
}

func (f *Function) Stop(_ context.Context) error {
	if f.recorderURL != "" {
		resp, err := http.Post(f.recorderURL+"/record?id=stop-go", "text/plain", nil)
		if err == nil {
			resp.Body.Close()
		}
	}
	return nil
}

func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "OK")
}
