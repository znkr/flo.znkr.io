package renderers

import (
	"bytes"
	"fmt"
	"html/template"

	"flo.znkr.io/generator/directives"
	"flo.znkr.io/generator/goldmark"
	"flo.znkr.io/generator/site"
)

type MarkdownRenderer struct {
	directives *directives.Renderer
	page       *template.Template
}

type MarkdownRendererOptions struct {
	PageTemplate string
}

func NewMarkdownRenderer(templates *template.Template, opts MarkdownRendererOptions) (*MarkdownRenderer, error) {
	page := templates.Lookup(opts.PageTemplate)
	if page == nil {
		return nil, fmt.Errorf("template not found %s", opts.PageTemplate)
	}

	return &MarkdownRenderer{
		page:       page,
		directives: directives.NewRenderer(templates),
	}, nil
}

func (r *MarkdownRenderer) RenderContent(s *site.Site, doc *site.Doc, data []byte) ([]byte, error) {
	content, err := goldmark.Render(data)
	if err != nil {
		return nil, err
	}

	content, err = r.directives.Render(doc, content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (r *MarkdownRenderer) RenderPage(s *site.Site, doc *site.Doc, data []byte) ([]byte, error) {
	content, err := r.RenderContent(s, doc, data)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = r.page.Execute(&buf, struct {
		Meta    *site.Metadata
		Site    *site.Site
		Content template.HTML
	}{
		Meta:    doc.Meta,
		Site:    s,
		Content: template.HTML(content),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}
