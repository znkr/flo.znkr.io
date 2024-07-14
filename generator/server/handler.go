package server

import (
	"errors"
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

	doc, err := s.Get(req.URL.EscapedPath())
	switch {
	case errors.Is(err, site.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
		return
	case err != nil:
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", doc.MimeType())
	w.WriteHeader(http.StatusOK)

	if req.Method == http.MethodHead {
		return
	}

	b, err := s.RenderPage(doc)
	if err != nil {
		panic(err)
	}

	if _, err := w.Write(b); err != nil {
		panic(err)
	}
}
