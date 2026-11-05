package oci

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	fn "knative.dev/func/pkg/functions"
)

// A client can reuse its pusher when a function is built and pushed again.
func TestPusher_Reuse(t *testing.T) {
	for _, tt := range []struct {
		name      string
		failFirst bool
	}{
		{name: "after successful upload"},
		{name: "after failed upload", failFirst: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			indexType, err := empty.Index.MediaType()
			if err != nil {
				t.Fatal(err)
			}
			var reject atomic.Bool
			reject.Store(tt.failFirst)
			handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && r.Header.Get("Content-Type") == string(indexType) && reject.Swap(false) {
					http.Error(w, "test index rejection", http.StatusBadRequest)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)

			f := fn.Function{Root: t.TempDir()}
			f.Build.Image = strings.TrimPrefix(server.URL, "http://") + "/funcs/reuse:latest"
			if _, err := layout.Write(filepath.Join(f.Root, fn.RunDataDir, fn.BuildDir, "oci"), empty.Index); err != nil {
				t.Fatal(err)
			}
			want, err := empty.Index.Digest()
			if err != nil {
				t.Fatal(err)
			}

			pusher := NewPusher(true, true, false)
			for i := 0; i < 2; i++ {
				got, err := pusher.Push(t.Context(), f)
				if i == 0 && tt.failFirst {
					if err == nil || !strings.Contains(err.Error(), "test index rejection") {
						t.Fatalf("first upload: expected registry rejection, got %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("push %d: %v", i+1, err)
				}
				if got != want.String() {
					t.Fatalf("push %d: digest = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}
