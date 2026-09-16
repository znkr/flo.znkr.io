package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sync"
	"time"
)

// Paths the reloader serves. They're checked before the site is consulted, so
// a document can never shadow them.
const (
	eventsPath = "/_dev/reload"
	scriptPath = "/_dev/reload.js"
)

//go:embed reload.js
var reloadScript []byte

var scriptTag = []byte(`<script src="` + scriptPath + `" defer></script>`)

// status is what the browser is told about the site: which build it is
// serving, and whether the build after it failed.
type status struct {
	// Version changes on every site change.
	Version int64 `json:"version"`
	// Warnings is what compiling the site served warned about, formatted one
	// per line, empty if nothing did.
	Warnings string `json:"warnings,omitempty"`
	// Error is why the last reload failed, empty if it succeeded. The site
	// served is the one from the last successful reload.
	Error string `json:"error,omitempty"`
}

// reloader tells connected browsers when the site changed.
type reloader struct {
	mu      sync.Mutex
	status  status
	changed chan struct{} // closed and replaced on every status change
	done    chan struct{} // closed when the server starts to shut down
}

func newReloader(warnings string) *reloader {
	return &reloader{
		status:  status{Version: time.Now().UnixNano(), Warnings: warnings},
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// notify tells every connected browser that the site changed and what
// compiling it warned about.
func (r *reloader) notify(warnings string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status{Version: time.Now().UnixNano(), Warnings: warnings}
	r.wakeLocked()
}

// fail tells every connected browser that rebuilding the site failed. The
// version is left alone: the site served did not change.
func (r *reloader) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Error = err.Error()
	r.wakeLocked()
}

// wakeLocked wakes every waiter. The caller holds mu.
func (r *reloader) wakeLocked() {
	// Closing and replacing the channel wakes every waiter at once, which
	// saves keeping track of who's connected.
	close(r.changed)
	r.changed = make(chan struct{})
}

// stop releases all connected browsers.
func (r *reloader) stop() {
	close(r.done)
}

func (r *reloader) state() (status, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status, r.changed
}

// serveEvents streams the status to the browser, once on connect and again
// whenever it changes.
func (r *reloader) serveEvents(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	for {
		s, changed := r.state()
		b, err := json.Marshal(s)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
		select {
		case <-changed:
		case <-req.Context().Done():
			return
		case <-r.done:
			return
		}
	}
}

func (r *reloader) serveScript(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if req.Method == http.MethodHead {
		return
	}
	w.Write(reloadScript)
}

// errorPage returns an HTML page showing err, for a document that failed to
// render.
func errorPage(err error) []byte {
	return []byte(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Error</title></head>
<body style="margin:0;background:#2a1414;color:#f8e0e0">
<pre style="margin:0;padding:1rem;font:14px/1.4 monospace;white-space:pre-wrap">` +
		html.EscapeString(err.Error()) + `</pre>
</body>
</html>
`)
}

// inject adds the reload script to an HTML page, just before the closing body
// tag, or at the end if there is none.
func inject(page []byte) []byte {
	i := bytes.LastIndex(page, []byte("</body>"))
	if i < 0 {
		i = len(page)
	}
	out := make([]byte, 0, len(page)+len(scriptTag))
	out = append(out, page[:i]...)
	out = append(out, scriptTag...)
	out = append(out, page[i:]...)
	return out
}
