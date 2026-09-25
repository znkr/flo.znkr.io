package renderers_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
	"github.com/google/go-cmp/cmp"
)

// TestRenderSitemap checks which documents a sitemap lists and what it says
// each was last modified at: a draft is left out, and an updated article is
// listed as of its update.
func TestRenderSitemap(t *testing.T) {
	entries := []renderers.Entry{
		{Path: "/", Meta: site.Metadata{CanonicalURL: "https://flo.znkr.io/"}},
		{Path: "/p", Meta: func() site.Metadata { m := article(); m.Updated = updated; return m }()},
		{Path: "/about", Meta: site.Metadata{Type: "page", CanonicalURL: "https://flo.znkr.io/about/"}},
		{Path: "/draft", Meta: site.Metadata{Type: "article", CanonicalURL: "https://flo.znkr.io/draft/"}},
		{Path: "/feed.atom", Meta: site.Metadata{}},
	}

	got, err := renderers.RenderSitemap(entries)
	if err != nil {
		t.Fatalf("RenderSitemap() = %v", err)
	}

	want := xml.Header +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
		`<url><loc>https://flo.znkr.io/</loc></url>` +
		`<url><loc>https://flo.znkr.io/about/</loc></url>` +
		`<url><loc>https://flo.znkr.io/p/</loc><lastmod>2025-03-01T00:00:00Z</lastmod></url>` +
		`</urlset>`
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("RenderSitemap(): -want +got:\n%s", diff)
	}
}

// TestRenderSitemapNeedsADocument checks that a site no document is indexed
// from is an error rather than an empty sitemap.
func TestRenderSitemapNeedsADocument(t *testing.T) {
	_, err := renderers.RenderSitemap([]renderers.Entry{
		{Path: "/draft", Meta: site.Metadata{Type: "article", CanonicalURL: "https://flo.znkr.io/draft/"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no documents") {
		t.Errorf("RenderSitemap() with nothing to list = %v, want an error", err)
	}
}
