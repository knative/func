package mock

import (
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"

	impl "github.com/google/go-containerregistry/pkg/registry"
)

type Registry struct {
	*httptest.Server

	HandlerFunc  http.HandlerFunc
	RegistryImpl http.Handler
}

func NewRegistry() *Registry {
	registryHandler := impl.New(impl.Logger(log.New(os.Stderr, "test regsitry: ", log.LstdFlags)))
	r := &Registry{
		RegistryImpl: registryHandler,
	}
	r.Server = httptest.NewServer(r)

	return r
}

func (r *Registry) Addr() net.Addr {
	return r.Server.Listener.Addr()
}

func (r *Registry) Close() {
	r.Server.Close()
}

func (r *Registry) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	if r.HandlerFunc != nil {
		r.HandlerFunc(res, req)
	} else {
		r.RegistryImpl.ServeHTTP(res, req)
	}
}
