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
	if doc == nil && s.Doc(req.URL.EscapedPath()+"/") != nil {
		// GitHub Pages serves a directory's index.html at the path with a
		// trailing slash and redirects the path without it, keeping the query.
		u := *req.URL
		u.Path += "/"
		u.RawPath = ""
		http.Redirect(w, req, u.RequestURI(), http.StatusMovedPermanently)
		return
	}
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
		log.Printf("failed to serve %v: %v", req.URL.EscapedPath(), err)
		// The error page carries the reload script, so that the page comes
		// back on its own once the document renders again.
		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(inject(errorPage(err)))
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
