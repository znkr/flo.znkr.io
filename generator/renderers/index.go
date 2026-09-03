// Package renderers holds the renderers for the two documents that aren't
// compiled from a markst source: the site index and the atom feed. Both are
// built from what the site's documents say about themselves.
package renderers

import (
	"bytes"
	"fmt"
	"html/template"
	"slices"

	"flo.znkr.io/generator/site"
)

// Entry is one document as a listing sees it: where it is, and what it says
// about itself.
//
// It is what a [site.Doc] holds once its metadata has been computed. The
// renderers take entries rather than documents because a document's metadata is
// an artifact, and a rule is given values.
type Entry struct {
	Path string
	Meta site.Metadata
}

// Articles returns the published articles among entries, newest first. Two
// articles published on the same day keep the order they came in.
func Articles(entries []Entry) []Entry {
	var ret []Entry
	for _, e := range entries {
		if e.Meta.Type != "article" || e.Meta.Published.IsZero() {
			continue
		}
		ret = append(ret, e)
	}
	slices.SortStableFunc(ret, func(a, b Entry) int {
		return b.Meta.Published.Compare(a.Meta.Published)
	})
	return ret
}

// RenderIndex renders the site index from entries, of which it lists the
// published articles.
//
// Every document is passed, not only the articles, because an edit can change
// which documents are articles.
func RenderIndex(templates *template.Template, title, goImport string, entries []Entry) ([]byte, error) {
	page := templates.Lookup("index")
	if page == nil {
		return nil, fmt.Errorf("template not found index")
	}

	var buf bytes.Buffer
	err := page.Execute(&buf, struct {
		Meta     site.Metadata
		Articles []Entry
	}{
		Meta:     site.Metadata{Title: title, GoImport: goImport},
		Articles: Articles(entries),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}
