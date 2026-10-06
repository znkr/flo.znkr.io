package renderers_test

import (
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"
	"time"

	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
	"github.com/google/go-cmp/cmp"
)

var (
	published = time.Date(2024, 7, 6, 0, 0, 0, 0, time.UTC)
	updated   = time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
)

// article returns the metadata of a published article. Each case changes the
// one field it is about.
func article() site.Metadata {
	return site.Metadata{
		Title:        "Title",
		Type:         "article",
		Published:    published,
		Updated:      published,
		Summary:      "A summary.",
		CanonicalURL: "https://flo.znkr.io/p/",
		Image:        "https://flo.znkr.io/p/card.png",
		ImageWidth:   1200,
		ImageHeight:  630,
	}
}

// page returns the metadata of a page, which says nothing about when it was
// written.
func page() site.Metadata {
	return site.Metadata{
		Title:        "Title",
		Type:         "page",
		CanonicalURL: "https://flo.znkr.io/p/",
	}
}

// newHead is NewHead, failing the test rather than returning an error.
func newHead(t *testing.T, m site.Metadata) renderers.Head {
	t.Helper()
	h, err := renderers.NewHead(m, renderers.Assets{})
	if err != nil {
		t.Fatalf("NewHead() = %v", err)
	}
	return h
}

// tagLines writes one line per meta tag, which reads and diffs better than the
// struct does.
func tagLines(h renderers.Head) []string {
	var ret []string
	for _, t := range h.Tags {
		ret = append(ret, fmt.Sprintf("%s=%s: %s", t.Attr, t.Name, t.Content))
	}
	return ret
}

// TestNewHeadTags checks which meta tags each kind of document sets.
func TestNewHeadTags(t *testing.T) {
	const common = "" +
		"name=author: Florian Zenker\n" +
		"name=fediverse:creator: @znkr@hachyderm.io\n" +
		"name=color-scheme: light\n" +
		"name=theme-color: #1d8fe1"

	tests := []struct {
		name string
		meta site.Metadata
		want string
	}{
		{
			name: "article",
			meta: article(),
			want: common + "\n" +
				"name=description: A summary.\n" +
				"property=og:site_name: flo.znkr.io\n" +
				"property=og:title: Title\n" +
				"property=og:type: article\n" +
				"property=og:locale: en_US\n" +
				"property=og:url: https://flo.znkr.io/p/\n" +
				"property=og:description: A summary.\n" +
				"property=og:image: https://flo.znkr.io/p/card.png\n" +
				"property=og:image:width: 1200\n" +
				"property=og:image:height: 630\n" +
				"property=og:image:alt: Title - flo.znkr.io\n" +
				"property=article:author: https://flo.znkr.io/about/\n" +
				"property=article:published_time: 2024-07-06T00:00:00Z\n" +
				"name=twitter:card: summary_large_image",
		},
		{
			// An updated article states both dates. One published and updated
			// on the same day states only the first.
			name: "updated article",
			meta: func() site.Metadata { m := article(); m.Updated = updated; return m }(),
			want: common + "\n" +
				"name=description: A summary.\n" +
				"property=og:site_name: flo.znkr.io\n" +
				"property=og:title: Title\n" +
				"property=og:type: article\n" +
				"property=og:locale: en_US\n" +
				"property=og:url: https://flo.znkr.io/p/\n" +
				"property=og:description: A summary.\n" +
				"property=og:image: https://flo.znkr.io/p/card.png\n" +
				"property=og:image:width: 1200\n" +
				"property=og:image:height: 630\n" +
				"property=og:image:alt: Title - flo.znkr.io\n" +
				"property=article:author: https://flo.znkr.io/about/\n" +
				"property=article:published_time: 2024-07-06T00:00:00Z\n" +
				"property=article:modified_time: 2025-03-01T00:00:00Z\n" +
				"name=twitter:card: summary_large_image",
		},
		{
			// A draft is served like every other document, so it is the one
			// that says it is not to be indexed, and it states no dates.
			name: "draft",
			meta: func() site.Metadata {
				m := article()
				m.Published, m.Updated = time.Time{}, time.Time{}
				return m
			}(),
			want: "name=robots: noindex\n" + common + "\n" +
				"name=description: A summary.\n" +
				"property=og:site_name: flo.znkr.io\n" +
				"property=og:title: Title\n" +
				"property=og:type: article\n" +
				"property=og:locale: en_US\n" +
				"property=og:url: https://flo.znkr.io/p/\n" +
				"property=og:description: A summary.\n" +
				"property=og:image: https://flo.znkr.io/p/card.png\n" +
				"property=og:image:width: 1200\n" +
				"property=og:image:height: 630\n" +
				"property=og:image:alt: Title - flo.znkr.io\n" +
				"property=article:author: https://flo.znkr.io/about/\n" +
				"name=twitter:card: summary_large_image",
		},
		{
			// A page is a website with no dates, no author and no card.
			name: "page",
			meta: page(),
			want: common + "\n" +
				"property=og:site_name: flo.znkr.io\n" +
				"property=og:title: Title\n" +
				"property=og:type: website\n" +
				"property=og:locale: en_US\n" +
				"property=og:url: https://flo.znkr.io/p/",
		},
		{
			// The index and the feed say what they say in the generator.
			name: "generated document",
			meta: site.Metadata{
				Title:    "Title",
				GoImport: "flo.znkr.io git https://github.com/znkr/flo.znkr.io",
				Redirect: "/elsewhere",
			},
			want: "name=go-import: flo.znkr.io git https://github.com/znkr/flo.znkr.io\n" +
				"http-equiv=refresh: 0;URL='/elsewhere'\n" + common + "\n" +
				"property=og:site_name: flo.znkr.io\n" +
				"property=og:title: Title\n" +
				"property=og:type: website\n" +
				"property=og:locale: en_US",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagLines(newHead(t, tt.meta))
			if diff := cmp.Diff(strings.Split(tt.want, "\n"), got); diff != "" {
				t.Errorf("NewHead().Tags: -want +got:\n%s", diff)
			}
		})
	}
}

// TestNewHeadLinks checks that every page links to the author's profiles, and
// that only a page with a URL states a canonical one.
func TestNewHeadLinks(t *testing.T) {
	profiles := []renderers.Link{
		{Rel: "author", Href: "https://flo.znkr.io/about/"},
		{Rel: "me", Href: "https://github.com/znkr"},
		{Rel: "me", Href: "https://hachyderm.io/@znkr"},
		{Rel: "me", Href: "https://bsky.app/profile/flo.znkr.io"},
	}

	want := append([]renderers.Link{{Rel: "canonical", Href: "https://flo.znkr.io/p/"}}, profiles...)
	if diff := cmp.Diff(want, newHead(t, article()).Links); diff != "" {
		t.Errorf("NewHead().Links: -want +got:\n%s", diff)
	}

	m := article()
	m.CanonicalURL = ""
	if diff := cmp.Diff(profiles, newHead(t, m).Links); diff != "" {
		t.Errorf("NewHead().Links without a canonical URL: -want +got:\n%s", diff)
	}
}

// nodes parses the JSON-LD of a head into one map per node, keyed by type.
func nodes(t *testing.T, h renderers.Head) ([]string, map[string]map[string]any) {
	t.Helper()
	var g struct {
		Context string           `json:"@context"`
		Graph   []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal([]byte(h.JSONLD), &g); err != nil {
		t.Fatalf("parsing JSON-LD %s: %v", h.JSONLD, err)
	}
	if g.Context != "https://schema.org" {
		t.Errorf("JSON-LD @context = %q, want the schema.org context", g.Context)
	}
	var types []string
	byType := make(map[string]map[string]any)
	for _, n := range g.Graph {
		typ, _ := n["@type"].(string)
		types = append(types, typ)
		byType[typ] = n
	}
	return types, byType
}

// TestNewHeadGraph checks which nodes each kind of document states.
func TestNewHeadGraph(t *testing.T) {
	tests := []struct {
		name string
		meta site.Metadata
		want []string
	}{
		{
			name: "article",
			meta: article(),
			want: []string{"WebSite", "Person", "WebPage", "BreadcrumbList", "Article"},
		},
		{
			name: "page",
			meta: page(),
			want: []string{"WebSite", "Person", "WebPage", "BreadcrumbList"},
		},
		{
			// The site root is the first step of every breadcrumb, so it has
			// none of its own.
			name: "site root",
			meta: func() site.Metadata { m := page(); m.CanonicalURL = "https://flo.znkr.io/"; return m }(),
			want: []string{"WebSite", "Person", "WebPage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types, _ := nodes(t, newHead(t, tt.meta))
			if diff := cmp.Diff(tt.want, types); diff != "" {
				t.Errorf("NewHead() states: -want +got:\n%s", diff)
			}
		})
	}

	// A document with no URL of its own has no node to state, because a node
	// is named by a URL.
	m := article()
	m.CanonicalURL = ""
	if got := newHead(t, m).JSONLD; got != "" {
		t.Errorf("NewHead().JSONLD without a canonical URL = %s, want none", got)
	}
}

// TestNewHeadGraphNames checks that the nodes of an article's graph name each
// other, which is what holds the author to one node for the whole site.
func TestNewHeadGraphNames(t *testing.T) {
	_, byType := nodes(t, newHead(t, article()))

	person := byType["Person"]["@id"]
	if want := "https://flo.znkr.io/about/#me"; person != want {
		t.Errorf("Person @id = %v, want %v", person, want)
	}
	if got, want := byType["Person"]["sameAs"], "https://hachyderm.io/@znkr"; !slicesContains(got, want) {
		t.Errorf("Person sameAs = %v, want it to hold %v", got, want)
	}

	a := byType["Article"]
	for _, field := range []string{"author", "publisher"} {
		if got := ref(a[field]); got != person {
			t.Errorf("Article %s = %v, want the Person node %v", field, got, person)
		}
	}
	if got, want := ref(a["mainEntityOfPage"]), byType["WebPage"]["@id"]; got != want {
		t.Errorf("Article mainEntityOfPage = %v, want the WebPage node %v", got, want)
	}
	if got, want := ref(a["isPartOf"]), byType["WebSite"]["@id"]; got != want {
		t.Errorf("Article isPartOf = %v, want the WebSite node %v", got, want)
	}
	if got, want := ref(byType["WebPage"]["breadcrumb"]), byType["BreadcrumbList"]["@id"]; got != want {
		t.Errorf("WebPage breadcrumb = %v, want the BreadcrumbList node %v", got, want)
	}
	if got, want := a["datePublished"], "2024-07-06T00:00:00Z"; got != want {
		t.Errorf("Article datePublished = %v, want %v", got, want)
	}

	// The breadcrumb leads from the site root to the page, and the step it
	// stands on leads nowhere.
	crumbs, _ := byType["BreadcrumbList"]["itemListElement"].([]any)
	if len(crumbs) != 2 {
		t.Fatalf("BreadcrumbList = %v, want two steps", byType["BreadcrumbList"]["itemListElement"])
	}
	first, _ := crumbs[0].(map[string]any)
	last, _ := crumbs[1].(map[string]any)
	if got, want := first["item"], "https://flo.znkr.io/"; got != want {
		t.Errorf("first breadcrumb item = %v, want %v", got, want)
	}
	if got, ok := last["item"]; ok {
		t.Errorf("last breadcrumb item = %v, want none", got)
	}
	if got, want := last["name"], "Title"; got != want {
		t.Errorf("last breadcrumb name = %v, want %v", got, want)
	}
}

// TestNewHeadDraftStatesNoDates checks that a draft says nothing about when it
// was published, in either representation, rather than claiming year one.
func TestNewHeadDraftStatesNoDates(t *testing.T) {
	m := article()
	m.Published, m.Updated = time.Time{}, time.Time{}

	_, byType := nodes(t, newHead(t, m))
	for _, field := range []string{"datePublished", "dateModified"} {
		if got, ok := byType["Article"][field]; ok {
			t.Errorf("Article %s = %v, want none", field, got)
		}
	}
}

// TestNewHeadEscapesTheJSONLD checks the property writing the JSON-LD into a
// script element unescaped rests on: marshaling escapes a title that would end
// the element.
func TestNewHeadEscapesTheJSONLD(t *testing.T) {
	m := article()
	m.Title = "</script><script>alert(1)</script>"

	if got := string(newHead(t, m).JSONLD); strings.Contains(got, "<") {
		t.Errorf("NewHead().JSONLD = %s, want the title's angle brackets escaped", got)
	}
}

func ref(v any) any {
	m, _ := v.(map[string]any)
	return m["@id"]
}

func slicesContains(v any, want string) bool {
	s, _ := v.([]any)
	for _, e := range s {
		if e == want {
			return true
		}
	}
	return false
}
