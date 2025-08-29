package directives

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const eof = -1

type Directive struct {
	Pos, End int
	Name     string
	Attrs    map[string]string
}

var ErrNotFound = errors.New("not found")

type SyntaxError struct {
	Msg       string
	Pos       int
	Line, Col int
}

func (err *SyntaxError) Error() string {
	return fmt.Sprintf("%s [%d:%d]", err.Msg, err.Line, err.Col)
}

func Parse(in []byte) (_ []Directive, err error) {
	defer func() {
		if e := recover(); e != nil {
			if e, ok := e.(*SyntaxError); ok {
				err = e
				return
			}
			panic(e)
		}
	}()

	p := parser{
		in: in,
	}

	var dirs []Directive
	for {
		dir, ok := p.parseNextDirective()
		if !ok {
			break
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

type parser struct {
	in []byte

	ch  rune
	chw int

	pos       int
	col, line int
	err       error
}

func (p *parser) parseNextDirective() (Directive, bool) {
	for {
		p.next()
		if p.ch == eof {
			break
		}

		pos := p.pos
		if p.ch == '<' && p.consume("<!--#") {
			d := Directive{}
			d.Pos = pos

			p.consumeSpaces()
			d.Name = p.parseIdent()

			d.Attrs = make(map[string]string)
			for {
				p.consumeSpaces()
				if !unicode.IsLetter(p.ch) {
					break
				}
				attr := p.parseIdent()
				if !p.consume("=") {
					p.errorf("unexpected %q, expected '='", p.ch)
				}
				value := p.parseValue()
				d.Attrs[attr] = value
			}

			if !p.consume("-->") {
				p.errorf("unexpected %q, expected '-->'", p.ch)
			}
			d.End = p.pos
			return d, true
		}
	}

	return Directive{}, false
}

func (p *parser) errorf(format string, args ...any) {
	panic(&SyntaxError{
		Msg:  fmt.Sprintf(format, args...),
		Pos:  p.pos,
		Line: p.line,
		Col:  p.col,
	})
}

func (p *parser) next() {
	p.pos += p.chw
	if p.pos >= len(p.in) {
		p.ch = eof
		p.chw = 0
		return
	}
	p.ch, p.chw = utf8.DecodeRune(p.in[p.pos:])
	if p.ch == utf8.RuneError {
		p.errorf("invalid UTF-8 [%d:%d]", p.line, p.col)
	}
	if p.ch == '\n' {
		p.line++
		p.col = 0
	}
	p.col++
}

func (p *parser) consumeSpaces() {
	for unicode.IsSpace(p.ch) {
		p.next()
	}
}

func (p *parser) consume(s string) bool {
	for _, r := range s {
		if p.ch != r {
			return false
		}
		p.next()
	}
	return true
}

func (p *parser) isnext(s string) bool {
	if len(p.in)-p.pos < len(s) {
		return false
	}
	return bytes.Equal(p.in[p.pos:p.pos+len(s)], []byte(s))
}

func (p *parser) parseIdent() string {
	if !unicode.IsLetter(p.ch) {
		p.errorf("unexpected %q, expected indentifier", p.ch)
	}
	pos := p.pos
	for unicode.IsLetter(p.ch) || unicode.IsDigit(p.ch) || p.ch == '-' {
		p.next()
	}
	return string(p.in[pos:p.pos])
}

func (p *parser) parseValue() string {
	if p.ch != '"' {
		p.errorf("unexpected %q, expected '\"'", p.ch)
	}
	p.next()

	if p.isnext("\"\"") {
		p.next()
		p.next()
		var sb strings.Builder
		for p.ch != '"' && !p.isnext("\"\"\"") && p.ch != eof {
			sb.WriteRune(p.ch)
			if p.ch == '\n' {
				for unicode.IsSpace(p.ch) {
					p.next()
				}
			} else {
				p.next()
			}
		}
		if p.ch == eof {
			p.errorf("unterminated tri-quoted string")
		}
		p.next()
		p.next()
		p.next()
		return sb.String()
	} else {
		pos := p.pos
		for p.ch != '"' && p.ch != eof && p.ch != '\n' {
			p.next()
		}
		if p.ch == eof || p.ch == '\n' {
			p.errorf("unterminated string")
			return ""
		}
		v := p.in[pos:p.pos]
		p.next()
		return string(v)
	}
}

func (d *Directive) HasAttr(name string) bool {
	_, ok := d.Attrs[name]
	return ok
}
