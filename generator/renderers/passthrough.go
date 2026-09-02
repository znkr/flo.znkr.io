package renderers

import "flo.znkr.io/generator/site"

var Passthrough site.Renderer = &passthroughRenderer{}

type passthroughRenderer struct{}

func (r *passthroughRenderer) RenderContent(_ *site.Site, doc *site.Doc) ([]byte, error) {
	return doc.RenderData.([]byte), nil
}

func (r *passthroughRenderer) RenderPage(_ *site.Site, doc *site.Doc) ([]byte, error) {
	return doc.RenderData.([]byte), nil
}
