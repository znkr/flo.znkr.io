package renderers

import (
	"bytes"
	"fmt"
	"html/template"

	"flo.znkr.io/generator/site"
)

func Chain(renderers ...site.Renderer) site.Renderer {
	var ret chainedRenderer
	for _, r := range renderers {
		switch r := r.(type) {
		case chainedRenderer:
			ret = append(ret, r...)
		case *passthroughRenderer:
			// do nothing
		default:
			ret = append(ret, r)
		}
	}
	return ret
}

var Passthrough site.Renderer = &passthroughRenderer{}

type passthroughRenderer struct{}

func (r *passthroughRenderer) Render(_ *site.Site, _ *site.Doc, data []byte) ([]byte, error) {
	return data, nil
}

type chainedRenderer []site.Renderer

func (r chainedRenderer) Render(site *site.Site, doc *site.Doc, data []byte) ([]byte, error) {
	for _, r := range r {
		var err error
		data, err = r.Render(site, doc, data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

type TemplateRenderer struct {
	template *template.Template
}

func NewTemplateRenderer(template *template.Template) *TemplateRenderer {
	return &TemplateRenderer{template: template}
}

func (r *TemplateRenderer) Render(s *site.Site, doc *site.Doc, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	err := r.template.Execute(&buf, struct {
		Meta    *site.Metadata
		Site    *site.Site
		Content template.HTML
	}{
		Meta:    doc.Meta,
		Site:    s,
		Content: template.HTML(data),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}
