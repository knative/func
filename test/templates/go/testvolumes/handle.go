package function

/*
This function template read and (optionally) write the content of a file on the server
The template is meant to be used in by `func config volumes` e2e test
*/
import (
	"fmt"
	"net/http"
	"os"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	// v=/test/volume-config/myconfig
	// w=hello
	path := r.URL.Query().Get("v")
	content := r.URL.Query().Get("w")

	if path != "" {
		if content != "" {
			f, err := os.Create(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating file: %v\n", err)
			} else {
				defer f.Close()
				err = os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
				}
			}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file: %v", err)
		}
		_, err = fmt.Fprintf(w, "%v", string(b))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error on response write: %v", err)
		}
	}

}
