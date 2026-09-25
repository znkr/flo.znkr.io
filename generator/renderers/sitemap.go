package renderers

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"slices"
	"time"

	"flo.znkr.io/generator/site"
)

// sitemapNS is the namespace a sitemap is written in.
const sitemapNS = "http://www.sitemaps.org/schemas/sitemap/0.9"

// RenderSitemap renders the sitemap from entries, of which it lists every
// document a search engine is to index: one with a URL of its own that is no
// draft.
//
// Every document is passed, not only the indexed ones, because an edit can
// change which documents those are.
func RenderSitemap(entries []Entry) ([]byte, error) {
	set := urlset{NS: sitemapNS}
	for _, e := range entries {
		if e.Meta.CanonicalURL == "" || e.Meta.Draft() {
			continue
		}
		u := sitemapURL{Loc: e.Meta.CanonicalURL}
		if t := lastModified(e.Meta); !t.IsZero() {
			u.LastMod = t.Format(time.RFC3339)
		}
		set.URLs = append(set.URLs, u)
	}
	if len(set.URLs) == 0 {
		return nil, fmt.Errorf("no documents to build a sitemap from")
	}
	slices.SortFunc(set.URLs, func(a, b sitemapURL) int { return cmp.Compare(a.Loc, b.Loc) })

	b, err := xml.Marshal(set)
	if err != nil {
		return nil, fmt.Errorf("encoding sitemap: %v", err)
	}
	return append([]byte(xml.Header), b...), nil
}

// lastModified returns when the document last changed, zero if it says nothing
// about when.
func lastModified(m site.Metadata) time.Time {
	if !m.Updated.IsZero() {
		return m.Updated
	}
	return m.Published
}

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}
