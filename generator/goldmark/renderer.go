package goldmark

import (
	"bytes"
	"fmt"

	"flo.znkr.io/generator/goldmark/admonitions"
	"flo.znkr.io/generator/site"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var Renderer site.Renderer = &renderer{}

type renderer struct{}

func (r *renderer) Render(_ *site.Site, _ *site.Doc, data []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			admonitions.Admonition,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	var buf bytes.Buffer
	if err := md.Convert(data, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %v", err)
	}

	return buf.Bytes(), nil
}
