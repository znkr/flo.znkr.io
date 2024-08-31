package renderers

import "flo.znkr.io/generator/site"

var Passthrough site.Renderer = &passthroughRenderer{}

type passthroughRenderer struct{}

func (r *passthroughRenderer) RenderContent(_ *site.Site, _ *site.Doc, data []byte) ([]byte, error) {
	return data, nil
}

func (r *passthroughRenderer) RenderPage(_ *site.Site, _ *site.Doc, data []byte) ([]byte, error) {
	return data, nil
}
