package server

import (
	"log"
	"net/http"
	"sync/atomic"

	"flo.znkr.io/generator/site"
)

type handler struct {
	site atomic.Pointer[site.Site]
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s := h.site.Load()

	switch req.Method {
	case http.MethodGet:
	case http.MethodHead:
	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	doc := s.Doc(req.URL.EscapedPath())
	if doc == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		if req.Method == http.MethodGet {
			w.Write([]byte("not found"))
		}
		return
	}

	w.Header().Set("Content-Type", doc.MimeType())
	if req.Method == http.MethodHead {
		return
	}

	b, err := s.RenderPage(doc)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		log.Printf("failed to serve %v: %v", req.URL.EscapedPath(), err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
