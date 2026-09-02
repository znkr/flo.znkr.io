package server

import (
	"bytes"
	_ "embed"
	"fmt"
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

// reloader tells connected browsers when the site changed.
type reloader struct {
	mu      sync.Mutex
	version int64         // changes on every site change
	changed chan struct{} // closed and replaced on every site change
	done    chan struct{} // closed when the server starts to shut down
}

func newReloader() *reloader {
	return &reloader{
		version: time.Now().UnixNano(),
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// notify tells every connected browser that the site changed.
func (r *reloader) notify() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version = time.Now().UnixNano()
	// Closing and replacing the channel wakes every waiter at once, which
	// saves keeping track of who's connected.
	close(r.changed)
	r.changed = make(chan struct{})
}

// stop releases all connected browsers.
func (r *reloader) stop() {
	close(r.done)
}

func (r *reloader) state() (int64, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version, r.changed
}

// serveEvents streams the site version to the browser, once on connect and
// again whenever the site changes.
func (r *reloader) serveEvents(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	for {
		version, changed := r.state()
		if _, err := fmt.Fprintf(w, "data: %d\n\n", version); err != nil {
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
