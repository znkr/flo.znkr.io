package admonitions

import (
	"fmt"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type Node struct {
	ast.BaseBlock
	Label string
}

var Kind = ast.NewNodeKind("Admonition")

func (n *Node) Kind() ast.NodeKind { return Kind }

func (n *Node) Dump(source []byte, level int) {
	m := map[string]string{}
	ast.DumpHelper(n, source, level, m, nil)
}

var Admonition goldmark.Extender = &admonitions{}

type admonitions struct{}

func (e *admonitions) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&blockParser{}, 999),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&nodeRenderer{}, 500),
		),
	)
}

type blockParser struct{}

var _ parser.BlockParser = (*blockParser)(nil)

func (p *blockParser) Trigger() []byte {
	return []byte{'N', 'T'}
}

var re = regexp.MustCompile("^(NOTE|TIP): ")

func (p *blockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}

	label := re.FindSubmatch(line[pos:])
	if label == nil {
		return nil, parser.NoChildren
	}
	pos += len(label[1]) + 2
	reader.Advance(pos)

	node := &Node{
		Label: string(label[1]),
	}
	return node, parser.HasChildren
}

func (p *blockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if util.IsBlank(line) {
		return parser.Close
	}
	reader.Advance(reader.LineOffset())
	return parser.Continue | parser.HasChildren
}

func (p *blockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
}

func (b *blockParser) CanInterruptParagraph() bool {
	return false
}

func (b *blockParser) CanAcceptIndentedLine() bool {
	return false
}

type nodeRenderer struct{}

var _ renderer.NodeRenderer = (*nodeRenderer)(nil)

func (r *nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(Kind, r.render)
}

func (r *nodeRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Node)
	var css, display string
	switch n.Label {
	case "TIP":
		css = "tip"
		display = "Tip"
	case "NOTE":
		css = "note"
		display = "Note"
	default:
		panic(fmt.Sprintf("unknown admonition label: %q", n.Label))
	}
	if entering {
		fmt.Fprintf(w, `<div class="admonition %s"><p class="admonition-title">%s</p>`, css, display)
	} else {
		w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}
