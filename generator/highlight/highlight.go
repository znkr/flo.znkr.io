package highlight

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"flo.znkr.io/generator/diff"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

var style = map[chroma.TokenType]string{
	chroma.Keyword:           "hl-b",
	chroma.KeywordPseudo:     "",
	chroma.KeywordType:       "",
	chroma.NameClass:         "hl-b",
	chroma.NameEntity:        "hl-b",
	chroma.NameException:     "hl-b",
	chroma.NameNamespace:     "hl-b",
	chroma.NameTag:           "hl-b",
	chroma.NameFunction:      "hl-bl",
	chroma.NameBuiltin:       "hl-bl",
	chroma.LiteralString:     "hl-i",
	chroma.OperatorWord:      "hl-b",
	chroma.Comment:           "hl-ii",
	chroma.CommentPreproc:    "",
	chroma.GenericEmph:       "hl-i",
	chroma.GenericHeading:    "hl-b",
	chroma.GenericPrompt:     "hl-b",
	chroma.GenericStrong:     "hl-b",
	chroma.GenericSubheading: "hl-b",
}

type Option func(*highlighter)

func Lang(lang string) Option {
	return func(o *highlighter) {
		o.lexer = lexers.Get(lang)
	}
}

func LangFromFilename(filename string) Option {
	return func(o *highlighter) {
		o.lexer = lexers.Match(filename)
	}
}

type Line struct {
	LineNo  int
	Content template.HTML
}

func Highlight(in string, opts ...Option) ([]Line, error) {
	hl := fromOptions(opts)
	lines, err := hl.lines(in)
	if err != nil {
		return nil, fmt.Errorf("parsing input: %v", err)
	}

	ret := make([]Line, 0, len(lines))
	for i, line := range lines {
		ret = append(ret, Line{i + 1, template.HTML(hl.highlight(line))})
	}
	return ret, nil
}

type Edit struct {
	Op      diff.Op
	XLineNo int
	YLineNo int
	Content template.HTML
}

func (ed *Edit) IsMatch() bool  { return ed.Op == diff.Match }
func (ed *Edit) IsDelete() bool { return ed.Op == diff.Delete }
func (ed *Edit) IsInsert() bool { return ed.Op == diff.Insert }

func Diff(a, b string, opts ...Option) ([]Edit, error) {
	hl := fromOptions(opts)

	hlalines, err := hl.lines(a)
	if err != nil {
		return nil, fmt.Errorf("parsing a: %v", err)
	}
	hlblines, err := hl.lines(b)
	if err != nil {
		return nil, fmt.Errorf("parsing b: %v", err)
	}

	var alines []string
	for _, l := range hlalines {
		var sb strings.Builder
		for _, t := range l {
			sb.WriteString(t.Value)
		}
		alines = append(alines, sb.String())
	}
	var blines []string
	for _, l := range hlblines {
		var sb strings.Builder
		for _, t := range l {
			sb.WriteString(t.Value)
		}
		blines = append(blines, sb.String())
	}
	edits := diff.Diff(alines, blines)

	ret := make([]Edit, 0, len(edits))
	s, t := 0, 0
	for _, edit := range edits {
		switch edit.Op {
		case diff.Match:
			ret = append(ret, Edit{edit.Op, s + 1, t + 1, template.HTML(hl.highlight(hlalines[s]))})
			s++
			t++
		case diff.Delete:
			ret = append(ret, Edit{edit.Op, s + 1, -1, template.HTML(hl.highlight(hlalines[s]))})
			s++
		case diff.Insert:
			ret = append(ret, Edit{edit.Op, -1, t + 1, template.HTML(hl.highlight(hlblines[t]))})
			t++
		}
	}
	return ret, nil
}

type highlighter struct {
	lexer chroma.Lexer
}

func fromOptions(opts []Option) *highlighter {
	hl := &highlighter{}
	for _, opt := range opts {
		opt(hl)
	}

	if hl.lexer == nil {
		hl.lexer = lexers.Fallback
	}
	hl.lexer = chroma.Coalesce(hl.lexer)
	return hl
}

func (hl *highlighter) highlight(line []chroma.Token) string {
	var sb strings.Builder
	for _, token := range line {
		class := class(token.Type)
		if class != "" {
			fmt.Fprintf(&sb, "<span class=\"%s\">", class)
		}
		sb.WriteString(html.EscapeString(token.Value))
		if class != "" {
			fmt.Fprintf(&sb, "</span>")
		}
	}
	return sb.String()
}

func (hl *highlighter) lines(in string) ([][]chroma.Token, error) {
	it, err := hl.lexer.Tokenise(nil, in)
	if err != nil {
		return nil, fmt.Errorf("creating iterator: %v", err)
	}
	return chroma.SplitTokensIntoLines(it.Tokens()), nil
}

func class(t chroma.TokenType) string {
	s, ok := style[t]
	if ok {
		return s
	}
	s, ok = style[t.SubCategory()]
	if ok {
		return s
	}
	s, ok = style[t.Category()]
	if ok {
		return s
	}
	return ""
}
