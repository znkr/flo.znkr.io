package server_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/server"
	"flo.znkr.io/generator/site"
)

// noMeta is the metadata of a test document, which says nothing about itself.
var noMeta = build.Const("meta", site.Metadata{})

// noErrors are the errors of a test document, which has none.
var noErrors = build.Const("errors", "")

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
			Meta:     noMeta,
			Errors:   noErrors,
			Page:     index,
		},
		{
			Path:     "/feed.atom",
			MimeType: "application/atom+xml;charset=utf-8",
			Meta:     noMeta,
			Errors:   noErrors,
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
		if err := s.Close(); err != nil {
			t.Errorf("Close() = %v", err)
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

func TestCloseEndsEventStreams(t *testing.T) {
	s, base := runServer(t)

	resp, err := http.Get(base + eventsPath)
	if err != nil {
		t.Fatalf("GET %s = %v", eventsPath, err)
	}
	defer resp.Body.Close()
	// Read the first event to make sure the stream is established and parked
	// before closing.
	r := bufio.NewReader(resp.Body)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("reading event: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Error("event stream ended without an error, want it cut off")
	}
}

func TestServeRedirectsToTrailingSlash(t *testing.T) {
	s, err := site.New([]site.Doc{{
		Path:     "/about/",
		MimeType: "text/html;charset=utf-8",
		Meta:     noMeta,
		Errors:   noErrors,
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

func TestServeDocumentWithErrors(t *testing.T) {
	s, err := site.New([]site.Doc{{
		Path:     "/about/",
		MimeType: "text/html;charset=utf-8",
		Meta:     noMeta,
		Errors:   build.Const("errors", "about.mst: boom"),
		Page:     page(pageBody),
	}})
	if err != nil {
		t.Fatalf("site.New() = %v", err)
	}
	_, base := runServerWith(t, s)

	resp, body := getStatus(t, base+"/about/")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("GET /about/ = %v, want 500", resp.Status)
	}
	if !strings.Contains(body, "<h1>hello</h1>") {
		t.Errorf("GET /about/ = %s, want the rendered page", body)
	}
}

func TestServeRevalidatesWithETag(t *testing.T) {
	_, base := runServer(t)

	resp, _ := getStatus(t, base+"/")
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("GET / sent no ETag")
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET / with If-None-Match = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("GET / with a matching If-None-Match = %v, want 304", resp.Status)
	}
}

func TestCloseClosesUnusedConnections(t *testing.T) {
	s, err := server.Run("localhost:0", build.NewCache(0), newSite(t), "")
	if err != nil {
		t.Fatalf("server.Run() = %v", err)
	}
	c, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatalf("Dial() = %v", err)
	}
	defer c.Close()
	// The connection sends no request, and the server tracks it only once it
	// has accepted it.
	time.Sleep(50 * time.Millisecond)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("Read() on a connection without a request = %v, want io.EOF", err)
	}
}
