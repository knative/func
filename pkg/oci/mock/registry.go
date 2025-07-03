package mock

import (
	"net"
	"net/http"
	"net/http/httptest"

	impl "github.com/google/go-containerregistry/pkg/registry"
)

type Registry struct {
	*httptest.Server

	HandlerFunc  http.HandlerFunc
	RegistryImpl http.Handler
}

func NewRegistry() *Registry {
	// TODO: this may be too excessive of logging, even for testing:
	// registryHandler := impl.New(impl.Logger(log.New(os.Stderr, "test registry: ", log.LstdFlags)))
	registryHandler := impl.New()
	r := &Registry{
		RegistryImpl: registryHandler,
	}
	r.Server = httptest.NewServer(r)

	return r
}

func (r *Registry) Addr() net.Addr {
	return r.Listener.Addr()
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
