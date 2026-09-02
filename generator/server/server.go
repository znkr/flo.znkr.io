package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"flo.znkr.io/generator/site"
)

// Server serves a single site via HTTP.
type Server struct {
	http    *http.Server
	addr    net.Addr
	handler *handler
	reload  *reloader
	errc    chan error
}

// Run creates a new server anc runs it in a new goroutine.
func Run(addr string, site *site.Site) (*Server, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("starting HTTP server: %v", err)
	}

	r := newReloader()

	h := &handler{reload: r}
	h.site.Store(site)

	s := &Server{
		http: &http.Server{
			Handler: h,
		},
		addr:    l.Addr(),
		handler: h,
		reload:  r,
		errc:    make(chan error),
	}

	// Shutdown waits for in-flight requests without cancelling them, so the
	// reload streams have to be released explicitly or shutting down blocks
	// for as long as a browser is connected.
	s.http.RegisterOnShutdown(r.stop)

	go func() {
		// ErrServerClosed is what a clean Shutdown looks like, not a failure.
		if err := s.http.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errc <- err
		}
	}()

	return s, nil
}

// ReplaceSite replaces the site to serve with the one provided and tells
// connected browsers to reload.
func (s *Server) ReplaceSite(site *site.Site) {
	s.handler.site.Store(site)
	s.reload.notify()
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() net.Addr {
	return s.addr
}

// Shutdown gracefully stops the sever.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down HTTP sever: %v", err)
	}
	return nil
}

// Error returns a channel to listen to errors while serving.
func (s *Server) Error() <-chan error {
	return s.errc
}
