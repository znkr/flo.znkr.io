// Package renderers holds every step that draws with the site's templates: a
// compiled document's body and the page around it, and the site index. It also
// holds the atom feed, the other document that is not compiled from a markst
// source. The index and the feed are built from what the site's documents say
// about themselves.
package renderers

import (
	"bytes"
	"fmt"
	"html/template"

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

// RenderIndex renders the site index, which lists articles in order and can
// load assets.
func RenderIndex(templates *template.Template, meta site.Metadata, assets Assets, articles []Entry) ([]byte, error) {
	page := templates.Lookup("index")
	if page == nil {
		return nil, fmt.Errorf("template not found index")
	}

	head, err := NewHead(meta, assets)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = page.Execute(&buf, struct {
		Meta     site.Metadata
		Head     Head
		Profiles []Profile
		Articles []Entry
	}{
		Meta:     meta,
		Head:     head,
		Profiles: authorProfiles,
		Articles: articles,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}
