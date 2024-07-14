package server

import (
	"context"
	"fmt"
	"net/http"

	"flo.znkr.io/generator/site"
)

// Server serves a single site via HTTP.
type Server struct {
	http    *http.Server
	handler *handler
}

// New creates a new server, but doesn't start it.
func New(addr string, site *site.Site) *Server {
	h := &handler{}
	h.site.Store(site)
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return &Server{
		http:    server,
		handler: h,
	}
}

// ReplaceSite replaces the site to serve with the one provided.
func (s *Server) ReplaceSite(site *site.Site) {
	s.handler.site.Store(site)
}

// Start starts the server, it blocks until [Shutdown] is called.
func (s *Server) Start() error {
	if err := s.http.ListenAndServe(); err != nil {
		return fmt.Errorf("starting HTTP server: %v", err)
	}
	return nil
}

// Shutdown gracefully stops the sever.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down HTTP sever: %v", err)
	}
	return nil
}
