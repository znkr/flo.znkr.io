package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"flo.znkr.io/generator/build"
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
//
// warnings is what compiling the site warned about, as for [Server.ReplaceSite].
func Run(addr string, c *build.Cache, site *site.Site, warnings string) (*Server, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("starting HTTP server: %v", err)
	}

	r := newReloader(warnings)

	h := &handler{cache: c, reload: r}
	h.site.Store(site)

	s := &Server{
		addr:    l.Addr(),
		handler: h,
		reload:  r,
		errc:    make(chan error),
	}
	s.http = &http.Server{Handler: h}

	go func() {
		// ErrServerClosed is what a clean Close looks like, not a failure.
		if err := s.http.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errc <- err
		}
	}()

	return s, nil
}

// ReplaceSite replaces the site to serve with the one provided and tells
// connected browsers to reload. warnings is what loading the site warned
// about, including documents whose metadata could not be computed, formatted
// for display. It is shown on every page until the next ReplaceSite.
func (s *Server) ReplaceSite(site *site.Site, warnings string) {
	s.handler.site.Store(site)
	s.reload.notify(warnings)
}

// ReportFailure tells connected browsers that rebuilding the site failed. The
// site served stays the one from the last successful ReplaceSite.
func (s *Server) ReportFailure(err error) {
	s.reload.fail(err)
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() net.Addr {
	return s.addr
}

// Close stops the server and closes every connection, the reload event
// streams and connections without a request included. Nothing waits on a
// request in flight: a page being served when the server stops is lost.
func (s *Server) Close() error {
	if err := s.http.Close(); err != nil {
		return fmt.Errorf("closing HTTP server: %v", err)
	}
	return nil
}

// Error returns a channel to listen to errors while serving.
func (s *Server) Error() <-chan error {
	return s.errc
}
