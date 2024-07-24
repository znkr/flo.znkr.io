package highlight

import (
	"fmt"
	"html"
	"html/template"
	"strings"

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

type Option func(*options)

type options struct {
	lexer chroma.Lexer
}

func Lang(lang string) Option {
	return func(o *options) {
		o.lexer = lexers.Get(lang)
	}
}

func Filename(filename string) Option {
	return func(o *options) {
		o.lexer = lexers.Match(filename)
	}
}

type Line struct {
	LineNo  int
	Content template.HTML
}

func Highlight(in string, opts ...Option) ([]Line, error) {
	options := options{}
	for _, opt := range opts {
		opt(&options)
	}

	if options.lexer == nil {
		options.lexer = lexers.Fallback
	}
	options.lexer = chroma.Coalesce(options.lexer)

	it, err := options.lexer.Tokenise(nil, in)
	if err != nil {
		return nil, fmt.Errorf("creating iterator: %v", err)
	}

	lines := chroma.SplitTokensIntoLines(it.Tokens())
	ret := make([]Line, 0, len(lines))
	for i, line := range lines {
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
		ret = append(ret, Line{i + 1, template.HTML(sb.String())})
	}
	return ret, nil
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
