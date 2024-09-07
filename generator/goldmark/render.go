package goldmark

import (
	"bytes"
	"fmt"

	"flo.znkr.io/generator/goldmark/admonitions"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/toc"
)

func Render(data []byte) ([]byte, []byte, error) {
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

	doc := md.Parser().Parse(text.NewReader(data))

	tree, err := toc.Inspect(doc, data, toc.MinDepth(2), toc.MaxDepth(2), toc.Compact(true))
	if err != nil {
		return nil, nil, fmt.Errorf("inspecting markdown doc for TOC: %v", err)
	}

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, data, doc); err != nil {
		return nil, nil, fmt.Errorf("rendering markdown: %v", err)
	}

	var tocbuf bytes.Buffer
	if list := toc.RenderList(tree); list != nil {
		// list will be nil if the table of contents is empty
		// because there were no headings in the document.
		md.Renderer().Render(&tocbuf, data, list)
	}

	return buf.Bytes(), tocbuf.Bytes(), nil
}
