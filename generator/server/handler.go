package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/site"
)

// instance tells this process's ETags from those of an earlier one.
var instance = strconv.FormatInt(time.Now().UnixNano(), 36)

type handler struct {
	site   atomic.Pointer[site.Site]
	cache  *build.Cache
	reload *reloader
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// The reload script and its event stream belong to serve, not to the
	// site, and the stream stays open for as long as the page does.
	switch req.URL.EscapedPath() {
	case eventsPath:
		h.reload.serveEvents(w, req)
		return
	case scriptPath:
		h.reload.serveScript(w, req)
		return
	}

	start := time.Now()
	status, errs := h.serveDoc(w, req)

	level := slog.LevelInfo
	switch {
	case status == http.StatusNotModified:
		// A browser revalidates every page and asset it loads, so these
		// would bury every other line.
		level = slog.LevelDebug
	case status >= 500:
		level = slog.LevelError
	case status >= 400:
		level = slog.LevelWarn
	}
	msg := req.Method + " " + req.URL.RequestURI()
	if errs != "" {
		msg += "\n" + errs
	}
	slog.Log(req.Context(), level, msg, "status", status, "took", time.Since(start))
}

// serveDoc serves the document at the request's path. It returns the status it
// responded with and the errors that made it fail, if any.
func (h *handler) serveDoc(w http.ResponseWriter, req *http.Request) (status int, errs string) {
	s := h.site.Load()

	switch req.Method {
	case http.MethodGet:
	case http.MethodHead:
	default:
		w.WriteHeader(http.StatusNotImplemented)
		return http.StatusNotImplemented, ""
	}

	doc := s.Doc(req.URL.EscapedPath())
	if doc == nil && s.Doc(req.URL.EscapedPath()+"/") != nil {
		// GitHub Pages serves a directory's index.html at the path with a
		// trailing slash and redirects the path without it, keeping the query.
		u := *req.URL
		u.Path += "/"
		u.RawPath = ""
		http.Redirect(w, req, u.RequestURI(), http.StatusMovedPermanently)
		return http.StatusMovedPermanently, ""
	}
	if doc == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		if req.Method == http.MethodGet {
			w.Write([]byte("not found"))
		}
		return http.StatusNotFound, ""
	}

	w.Header().Set("Content-Type", doc.MimeType)

	// The page's key is a hash of everything it is computed from, so it is
	// known without rendering the page and changes with any edit to it. A key
	// holds only within one process, which instance tells apart.
	etag := `"` + doc.Page.Key().String() + "-" + instance + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if req.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return http.StatusNotModified, ""
	}

	if req.Method == http.MethodHead {
		return http.StatusOK, ""
	}

	b, err := doc.Page.Get(req.Context(), h.cache)
	if err != nil {
		// The error page carries the reload script, so that the page comes
		// back on its own once the document renders again.
		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(inject(errorPage(err)))
		return http.StatusInternalServerError, err.Error()
	}

	if strings.HasPrefix(doc.MimeType, "text/html") {
		b = inject(b)
	}

	status = http.StatusOK
	// A document with errors renders to a page that shows them. It is served
	// with the status of a failure, like a page that did not render.
	if e, err := doc.Errors.Get(req.Context(), h.cache); err == nil && e != "" {
		status, errs = http.StatusInternalServerError, e
	}
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		slog.Warn("failed to write response", "path", req.URL.EscapedPath(), "err", err)
	}
	return status, errs
}
