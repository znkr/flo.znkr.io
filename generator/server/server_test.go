package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/server"
	"flo.znkr.io/generator/site"
)

// page returns an artifact holding a fixed body.
func page(body string) build.Artifact[[]byte] {
	return build.Const("test.page", []byte(body))
}

const (
	pageBody = "<html><body><h1>hello</h1></body></html>"
	feedBody = "<feed></feed>"
)

// failing returns an artifact that cannot be computed.
func failing(msg string) build.Artifact[[]byte] {
	return build.Derive[[]byte]("test.fail", func(msg string) ([]byte, error) { return nil, errors.New(msg) }, msg)
}

func newSite(t *testing.T) *site.Site {
	t.Helper()
	return siteWithIndex(t, page(pageBody))
}

// siteWithIndex returns a site whose / is index.
func siteWithIndex(t *testing.T, index build.Artifact[[]byte]) *site.Site {
	t.Helper()
	s, err := site.New([]site.Doc{
		{
			Path:     "/",
			MimeType: "text/html;charset=utf-8",
			Page:     index,
		},
		{
			Path:     "/feed.atom",
			MimeType: "application/atom+xml;charset=utf-8",
			Page:     page(feedBody),
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
	return runServerWith(t, newSite(t))
}

func runServerWith(t *testing.T, site *site.Site) (*server.Server, string) {
	t.Helper()
	s, err := server.Run("localhost:0", build.NewCache(0), site, "")
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
	resp, body := getStatus(t, url)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %v, want 200", url, resp.Status)
	}
	return body
}

// getStatus returns the response to a GET, whatever its status, and its body.
func getStatus(t *testing.T, url string) (*http.Response, string) {
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
	return resp, string(b)
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

// status is the event the reload stream carries.
type status struct {
	Version  int64  `json:"version"`
	Warnings string `json:"warnings"`
	Error    string `json:"error"`
}

// openEvents opens the reload stream and returns a function that reads the
// next event from it.
func openEvents(t *testing.T, base string) func() status {
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
	return func() status {
		t.Helper()
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("reading event: %v", err)
			}
			if data, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "data: "); ok {
				var s status
				if err := json.Unmarshal([]byte(data), &s); err != nil {
					t.Fatalf("decoding event %q: %v", data, err)
				}
				return s
			}
		}
	}
}

func TestReplaceSiteNotifiesBrowser(t *testing.T) {
	s, base := runServer(t)

	next := openEvents(t, base)
	first := next()

	s.ReplaceSite(newSite(t), "careful")

	second := next()
	if second.Version == first.Version {
		t.Errorf("version did not change after ReplaceSite, both %d", second.Version)
	}
	if second.Warnings != "careful" {
		t.Errorf("Warnings = %q after ReplaceSite, want %q", second.Warnings, "careful")
	}
}

func TestReportFailureReachesBrowser(t *testing.T) {
	s, base := runServer(t)

	next := openEvents(t, base)
	first := next()

	s.ReportFailure(errors.New("boom"))

	failed := next()
	if failed.Version != first.Version {
		t.Errorf("version changed after ReportFailure: %d, want %d", failed.Version, first.Version)
	}
	if failed.Error != "boom" {
		t.Errorf("Error = %q after ReportFailure, want %q", failed.Error, "boom")
	}

	s.ReplaceSite(newSite(t), "")

	fixed := next()
	if fixed.Version == first.Version {
		t.Errorf("version did not change after ReplaceSite, both %d", fixed.Version)
	}
	if fixed.Error != "" {
		t.Errorf("Error = %q after ReplaceSite, want it cleared", fixed.Error)
	}
}

func TestServeRendersErrorPage(t *testing.T) {
	_, base := runServerWith(t, siteWithIndex(t, failing("boom")))

	resp, body := getStatus(t, base+"/")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("GET / = %v, want 500", resp.Status)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", got)
	}
	for _, want := range []string{"boom", `<script src="/_dev/reload.js" defer></script>`} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / does not contain %q:\n%s", want, body)
		}
	}
}

func TestShutdownReleasesEventStreams(t *testing.T) {
	s, err := server.Run("localhost:0", build.NewCache(0), newSite(t), "")
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

func TestServeRedirectsToTrailingSlash(t *testing.T) {
	s, err := site.New([]site.Doc{{
		Path:     "/about/",
		MimeType: "text/html;charset=utf-8",
		Page:     page(pageBody),
	}})
	if err != nil {
		t.Fatalf("site.New() = %v", err)
	}
	_, base := runServerWith(t, s)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(base + "/about")
	if err != nil {
		t.Fatalf("GET /about = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/about/" {
		t.Errorf("GET /about = %v to %q, want 301 to /about/", resp.Status, resp.Header.Get("Location"))
	}
	get(t, base+"/about/")

	resp, err = client.Get(base + "/about?x=1")
	if err != nil {
		t.Fatalf("GET /about?x=1 = %v", err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Location"); got != "/about/?x=1" {
		t.Errorf("GET /about?x=1 redirects to %q, want /about/?x=1", got)
	}
}
