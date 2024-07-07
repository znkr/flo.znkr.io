package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"flo.znkr.io/generator/site"
)

type Server struct {
	http    *http.Server
	handler *proxyHandler
}

func New(addr string, site *site.Site) *Server {
	h := new(proxyHandler)
	h.set(&handler{
		site: site,
	})
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return &Server{
		http:    server,
		handler: h,
	}
}

func (s *Server) SetSite(site *site.Site) {
	s.handler.set(&handler{site: site})
}

func (s *Server) Start() error {
	if err := s.http.ListenAndServe(); err != nil {
		return fmt.Errorf("starting HTTP server: %v", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down HTTP sever: %v", err)
	}
	return nil
}

type proxyHandler struct {
	mu      sync.Mutex
	handler http.Handler
}

func (h *proxyHandler) set(handler http.Handler) {
	h.mu.Lock()
	h.handler = handler
	h.mu.Unlock()
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.mu.Lock()
	hh := h.handler
	h.mu.Unlock()
	hh.ServeHTTP(w, req)
}
