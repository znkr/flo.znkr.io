package highlight

import (
	"fmt"
	"html"
	"html/template"
	"slices"
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

func Diff(a, b string, opts ...Option) ([]diff.Edit[Line], error) {
	hl := fromOptions(opts)
	alines, err := hl.lines(a)
	if err != nil {
		return nil, fmt.Errorf("parsing a: %v", err)
	}
	blines, err := hl.lines(b)
	if err != nil {
		return nil, fmt.Errorf("parsing b: %v", err)
	}

	edits := diff.Diff(alines, blines, func(xs, ys []chroma.Token) bool { return slices.Equal(xs, ys) })

	ret := make([]diff.Edit[Line], 0, len(edits))
	s, t := 1, 1
	for _, edit := range edits {
		switch edit.Op {
		case diff.Match:
			s++
			t++
			ret = append(ret, diff.Edit[Line]{
				Op: edit.Op,
				X:  Line{s, template.HTML(hl.highlight(edit.X))},
				Y:  Line{t, template.HTML(hl.highlight(edit.Y))},
			})
		case diff.Delete:
			s++
			ret = append(ret, diff.Edit[Line]{
				Op: edit.Op,
				X:  Line{s, template.HTML(hl.highlight(edit.X))},
			})
		case diff.Insert:
			t++
			ret = append(ret, diff.Edit[Line]{
				Op: edit.Op,
				Y:  Line{t, template.HTML(hl.highlight(edit.Y))},
			})
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
