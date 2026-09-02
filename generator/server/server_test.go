package server_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"flo.znkr.io/generator/server"
	"flo.znkr.io/generator/site"
)

// testRenderer renders a fixed body, ignoring the site and document.
type testRenderer string

func (r testRenderer) RenderContent(*site.Site, *site.Doc) ([]byte, error) {
	return []byte(r), nil
}

func (r testRenderer) RenderPage(*site.Site, *site.Doc) ([]byte, error) {
	return []byte(r), nil
}

const (
	pageBody = "<html><body><h1>hello</h1></body></html>"
	feedBody = "<feed></feed>"
)

func newSite(t *testing.T) *site.Site {
	t.Helper()
	s, err := site.New([]site.Doc{
		{
			Path:     "/",
			MimeType: "text/html;charset=utf-8",
			Renderer: testRenderer(pageBody),
		},
		{
			Path:     "/feed.atom",
			MimeType: "application/atom+xml;charset=utf-8",
			Renderer: testRenderer(feedBody),
		},
	})
	if err != nil {
		t.Fatalf("site.New() = %v", err)
	}
	return s
}

// runServer starts a server on a free port and shuts it down when the test
// ends. It returns the base URL to make requests against.
func runServer(t *testing.T) (*server.Server, string) {
	t.Helper()
	s, err := server.Run("localhost:0", newSite(t))
	if err != nil {
		t.Fatalf("server.Run() = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() = %v", err)
		}
	})
	return s, "http://" + s.Addr().String()
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s = %v", url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %v, want 200", url, resp.Status)
	}
	return string(b)
}

func TestServeInjectsReloadScript(t *testing.T) {
	_, base := runServer(t)

	got := get(t, base+"/")
	want := `<html><body><h1>hello</h1><script src="/_dev/reload.js" defer></script></body></html>`
	if got != want {
		t.Errorf("GET / =\n%s\nwant:\n%s", got, want)
	}

	if script := get(t, base+"/_dev/reload.js"); !strings.Contains(script, "EventSource") {
		t.Errorf("GET /_dev/reload.js does not look like the reload script:\n%s", script)
	}
}

func TestServeLeavesNonHTMLAlone(t *testing.T) {
	_, base := runServer(t)

	if got := get(t, base+"/feed.atom"); got != feedBody {
		t.Errorf("GET /feed.atom = %q, want %q", got, feedBody)
	}
}

const eventsPath = "/_dev/reload"

// openEvents opens the reload stream and returns a function that reads the
// next event from it.
func openEvents(t *testing.T, base string) func() string {
	t.Helper()
	resp, err := http.Get(base + eventsPath)
	if err != nil {
		t.Fatalf("GET %s = %v", eventsPath, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	r := bufio.NewReader(resp.Body)
	return func() string {
		t.Helper()
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("reading event: %v", err)
			}
			if data, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "data: "); ok {
				return data
			}
		}
	}
}

func TestReplaceSiteNotifiesBrowser(t *testing.T) {
	s, base := runServer(t)

	next := openEvents(t, base)
	first := next()

	s.ReplaceSite(newSite(t))

	if second := next(); second == first {
		t.Errorf("version did not change after ReplaceSite, both %q", second)
	}
}

func TestShutdownReleasesEventStreams(t *testing.T) {
	s, err := server.Run("localhost:0", newSite(t))
	if err != nil {
		t.Fatalf("server.Run() = %v", err)
	}
	base := "http://" + s.Addr().String()

	// Read the first event to make sure the stream is established and parked
	// before shutting down.
	next := openEvents(t, base)
	next()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() = %v, want the open event stream to be released", err)
	}
}
