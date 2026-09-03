package server

import (
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/site"
)

type handler struct {
	site   atomic.Pointer[site.Site]
	cache  *build.Cache
	reload *reloader
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

	switch req.URL.EscapedPath() {
	case eventsPath:
		h.reload.serveEvents(w, req)
		return
	case scriptPath:
		h.reload.serveScript(w, req)
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

	w.Header().Set("Content-Type", doc.MimeType)
	if req.Method == http.MethodHead {
		return
	}

	b, err := doc.Page.Get(req.Context(), h.cache)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		log.Printf("failed to serve %v: %v", req.URL.EscapedPath(), err)
		return
	}

	if strings.HasPrefix(doc.MimeType, "text/html") {
		b = inject(b)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
