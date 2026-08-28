package renderers

import (
	"bytes"
	"fmt"
	"html/template"

	"flo.znkr.io/generator/site"
)

// NewIndexRenderer returns a renderer for the site index. The index has no
// source of its own: the template builds the page from the site's articles.
func NewIndexRenderer(templates *template.Template) (site.Renderer, error) {
	page := templates.Lookup("index")
	if page == nil {
		return nil, fmt.Errorf("template not found index")
	}
	return &indexRenderer{page: page}, nil
}

type indexRenderer struct {
	page *template.Template
}

func (r *indexRenderer) RenderContent(s *site.Site, doc *site.Doc) ([]byte, error) {
	return nil, fmt.Errorf("rendering content for the index is not possible")
}

func (r *indexRenderer) RenderPage(s *site.Site, doc *site.Doc) ([]byte, error) {
	var buf bytes.Buffer
	err := r.page.Execute(&buf, struct {
		Meta *site.Metadata
		Site *site.Site
	}{
		Meta: doc.Meta,
		Site: s,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}
