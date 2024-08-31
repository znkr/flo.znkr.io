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
)

func Render(data []byte) ([]byte, error) {
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
