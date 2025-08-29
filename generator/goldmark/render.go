package goldmark

import (
	"bytes"
	"fmt"

	"flo.znkr.io/generator/goldmark/admonitions"
	treeblood "github.com/wyatt915/goldmark-treeblood"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/toc"
)

func Render(data []byte) ([]byte, []byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			extension.Table,
			admonitions.Extension,
			treeblood.MathML(),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(&customRenderer{}, 999)),
		),
	)

	doc := md.Parser().Parse(text.NewReader(data))

	tree, err := toc.Inspect(doc, data, toc.MinDepth(2), toc.MaxDepth(4), toc.Compact(true))
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

type customRenderer struct{}

var _ renderer.NodeRenderer = (*customRenderer)(nil)

func (r *customRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
}

func (r *customRenderer) renderHeading(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		w.WriteString("<h")
		w.WriteByte("0123456"[n.Level])
		if n.Attributes() != nil {
			html.RenderAttributes(w, node, html.HeadingAttributeFilter)
		}
		w.WriteByte('>')
	} else {
		id, _ := n.AttributeString("id")
		w.WriteString("<a href=\"#")
		w.Write(id.([]byte))
		w.WriteString("\" class=\"anchor-link\"></a>")
		w.WriteString("</h")
		w.WriteByte("0123456"[n.Level])
		w.WriteString(">\n")
	}
	return ast.WalkContinue, nil
}
