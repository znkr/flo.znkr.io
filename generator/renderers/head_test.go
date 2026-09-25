package renderers_test

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
	"github.com/google/go-cmp/cmp"
)

// tag builds one expected meta tag.
func tag(attr, name, content string) renderers.Tag {
	return renderers.Tag{Attr: template.HTMLAttr(attr), Name: name, Content: content}
}

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
	}
}

// TestNewHead checks the two representations of a document's metadata against
// what each condition in it is supposed to produce.
func TestNewHead(t *testing.T) {
	const author = `[{"@context":"https://schema.org","@type":"Person","name":"Florian Zenker","url":"https://flo.znkr.io/about"}]`

	tests := []struct {
		name   string
		meta   site.Metadata
		tags   []renderers.Tag
		jsonld string
	}{
		{
			name: "article",
			meta: article(),
			tags: []renderers.Tag{
				tag("property", "og:site_name", "flo.znkr.io"),
				tag("property", "og:title", "Title"),
				tag("property", "og:type", "article"),
				tag("property", "og:image", "https://flo.znkr.io/p/card.png"),
				tag("name", "twitter:card", "summary_large_image"),
				tag("property", "og:url", "https://flo.znkr.io/p/"),
				tag("property", "article:author", "https://flo.znkr.io/about"),
				tag("property", "article:published_time", "2024-07-06"),
				tag("name", "description", "A summary."),
				tag("property", "og:description", "A summary."),
			},
			jsonld: `{"@context":"https://schema.org","@type":"Article","headline":"Title","author":` + author +
				`,"datePublished":"2024-07-06T00:00:00Z","dateModified":"2024-07-06T00:00:00Z"` +
				`,"image":"https://flo.znkr.io/p/card.png","url":"https://flo.znkr.io/p/"}`,
		},
		{
			name: "updated article",
			meta: func() site.Metadata { m := article(); m.Updated = updated; return m }(),
			tags: []renderers.Tag{
				tag("property", "og:site_name", "flo.znkr.io"),
				tag("property", "og:title", "Title"),
				tag("property", "og:type", "article"),
				tag("property", "og:image", "https://flo.znkr.io/p/card.png"),
				tag("name", "twitter:card", "summary_large_image"),
				tag("property", "og:url", "https://flo.znkr.io/p/"),
				tag("property", "article:author", "https://flo.znkr.io/about"),
				tag("property", "article:published_time", "2024-07-06"),
				tag("property", "article:modified_time", "2025-03-01"),
				tag("name", "description", "A summary."),
				tag("property", "og:description", "A summary."),
			},
			jsonld: `{"@context":"https://schema.org","@type":"Article","headline":"Title","author":` + author +
				`,"datePublished":"2024-07-06T00:00:00Z","dateModified":"2025-03-01T00:00:00Z"` +
				`,"image":"https://flo.znkr.io/p/card.png","url":"https://flo.znkr.io/p/"}`,
		},
		{
			// An article without a published date says nothing about when it
			// was published, in either representation.
			name: "unpublished article",
			meta: func() site.Metadata {
				m := article()
				m.Published, m.Updated = time.Time{}, time.Time{}
				return m
			}(),
			tags: []renderers.Tag{
				tag("property", "og:site_name", "flo.znkr.io"),
				tag("property", "og:title", "Title"),
				tag("property", "og:type", "article"),
				tag("property", "og:image", "https://flo.znkr.io/p/card.png"),
				tag("name", "twitter:card", "summary_large_image"),
				tag("property", "og:url", "https://flo.znkr.io/p/"),
				tag("property", "article:author", "https://flo.znkr.io/about"),
				tag("name", "description", "A summary."),
				tag("property", "og:description", "A summary."),
			},
			jsonld: `{"@context":"https://schema.org","@type":"Article","headline":"Title","author":` + author +
				`,"image":"https://flo.znkr.io/p/card.png","url":"https://flo.znkr.io/p/"}`,
		},
		{
			// A page is a website with no dates, no author and no JSON-LD.
			name: "page",
			meta: site.Metadata{
				Title:        "Title",
				Type:         "page",
				CanonicalURL: "https://flo.znkr.io/p/",
				Image:        "https://flo.znkr.io/p/card.png",
			},
			tags: []renderers.Tag{
				tag("property", "og:site_name", "flo.znkr.io"),
				tag("property", "og:title", "Title"),
				tag("property", "og:type", "website"),
				tag("property", "og:image", "https://flo.znkr.io/p/card.png"),
				tag("name", "twitter:card", "summary_large_image"),
				tag("property", "og:url", "https://flo.znkr.io/p/"),
			},
		},
		{
			// The index and the feed say what they say in the generator, and
			// have neither a canonical URL nor a summary.
			name: "generated document",
			meta: site.Metadata{
				Title:    "Title",
				GoImport: "flo.znkr.io git https://github.com/znkr/flo.znkr.io",
				Redirect: "/elsewhere",
			},
			tags: []renderers.Tag{
				tag("name", "go-import", "flo.znkr.io git https://github.com/znkr/flo.znkr.io"),
				tag("http-equiv", "refresh", "0;URL='/elsewhere'"),
				tag("property", "og:site_name", "flo.znkr.io"),
				tag("property", "og:title", "Title"),
				tag("property", "og:type", "website"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head, err := renderers.NewHead(tt.meta)
			if err != nil {
				t.Fatalf("NewHead() = %v", err)
			}
			if diff := cmp.Diff(tt.tags, head.Tags); diff != "" {
				t.Errorf("NewHead().Tags: -want +got:\n%s", diff)
			}
			if got := string(head.JSONLD); got != tt.jsonld {
				t.Errorf("NewHead().JSONLD =\n%s\nwant:\n%s", got, tt.jsonld)
			}
			if head.Title != tt.meta.Title || head.CanonicalURL != tt.meta.CanonicalURL {
				t.Errorf("NewHead() = title %q, canonical URL %q, want %q and %q",
					head.Title, head.CanonicalURL, tt.meta.Title, tt.meta.CanonicalURL)
			}
		})
	}
}

// TestNewHeadEscapesTheJSONLD checks the property writing the JSON-LD into a
// script element unescaped rests on: marshaling escapes a title that would end
// the element.
func TestNewHeadEscapesTheJSONLD(t *testing.T) {
	m := article()
	m.Title = "</script><script>alert(1)</script>"

	head, err := renderers.NewHead(m)
	if err != nil {
		t.Fatalf("NewHead() = %v", err)
	}
	if got := string(head.JSONLD); strings.Contains(got, "<") {
		t.Errorf("NewHead().JSONLD = %s, want the title's angle brackets escaped", got)
	}
}
