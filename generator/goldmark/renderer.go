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
	"github.com/yuin/goldmark/text"
)

var Renderer site.Renderer = &renderer{}

type renderer struct{}

func (r *renderer) Render(_ *site.Site, _ *site.Doc, data []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			admonitions.Extension,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	root := md.Parser().Parse(text.NewReader(data))

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, data, root); err != nil {
		return nil, fmt.Errorf("rendering markdown: %v", err)
	}

	return buf.Bytes(), nil
}
