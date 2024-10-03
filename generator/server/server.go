package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"flo.znkr.io/generator/site"
)

// Server serves a single site via HTTP.
type Server struct {
	http    *http.Server
	handler *handler
	errc    chan error
}

// Run creates a new server anc runs it in a new goroutine.
func Run(addr string, site *site.Site) (*Server, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("starting HTTP server: %v", err)
	}

	h := &handler{}
	h.site.Store(site)

	s := &Server{
		http: &http.Server{
			Handler: h,
		},
		handler: h,
		errc:    make(chan error),
	}

	go func() {
		if err := s.http.Serve(l); err != nil {
			s.errc <- err
		}
	}()

	return s, nil
}

// ReplaceSite replaces the site to serve with the one provided.
func (s *Server) ReplaceSite(site *site.Site) {
	s.handler.site.Store(site)
}

// Shutdown gracefully stops the sever.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down HTTP sever: %v", err)
	}
	close(s.errc)
	return nil
}

// Error returns a channel to listen to errors while serving.
func (s *Server) Error() <-chan error {
	return s.errc
}
