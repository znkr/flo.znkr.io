// Package builtins holds the markst functions the generator provides to every
// document, as opposed to the ones written in markst under lib, and the
// highlighting the documents they produce still need.
//
// The functions do not highlight anything. They read the files they are given
// and hand on a [Request] for the work, which the build runs as an artifact of
// its own so that an unchanged fragment is highlighted once however often the
// document around it changes. See [Artifacts] and [Artifact].
package builtins

import (
	"bytes"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"flo.znkr.io/generator/highlight"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Element names the markst elements this package produces. The presenter
// switches on the fragment type rather than on the name, so these are only what
// a diagnostic or a document dump calls them.
const (
	SnippetElem = "include-snippet"
	DiffElem    = "include-diff"
)

// The named parameters the two functions take, interned once.
var (
	nA       = name.Make("a")
	nB       = name.Make("b")
	nDisplay = name.Make("display")
	nLang    = name.Make("lang")
	nLines   = name.Make("lines")
)

// Include is what fragments/include_snippet and fragments/include_diff render,
// carried by the [value.Custom] the two functions produce: what is drawn around
// a fragment, and the request that computes it.
type Include struct {
	// File is the caption's label -- the display name, which defaults to the
	// path the include was read from.
	File string

	// FilePath is the caption link's target, relative to the site root.
	FilePath string

	// From and To are the line range to show, as indices into the highlighted
	// lines. To is -1 for a range that runs to the end. A diff shows every
	// line.
	From, To int

	// Req is what has to be highlighted before the include can be drawn.
	Req Request
}

// Bindings returns the markst bindings a document gets, reading the files it
// includes from vfs.
//
// vfs is per-document -- it is rooted at the directory the document was read
// from, which is what makes an include path relative to the document -- so
// these are built per compile rather than shared through a markst library.
//
// The functions read the file, resolve the lexer and parse the line range while
// the document is compiling, and hand the rest to the presenter inside a
// [value.Custom]. That is what makes a missing file or a malformed line range a
// compile error pointing at the argument that caused it, rather than a failure
// much later with nothing to point at.
func Bindings(vfs fs.FS) map[name.Name]value.Value {
	inc := &includes{fs: vfs}
	str := types.SetOf(types.Str)
	optStr := types.SetOf(types.Str, types.None)

	// No parameter carries a default: an argument that wasn't written is one
	// Lookup reports as absent, which is what the two functions branch on.
	snippet := &value.Function{
		Name:       SnippetElem,
		Positional: []value.Param{{Name: "file", Type: str}},
		Named: value.NamedParams{
			nLines:   {Name: "lines", Type: str},
			nDisplay: {Name: "display", Type: str},
			nLang:    {Name: "lang", Type: str},
		},
		F: inc.snippet,
	}

	diff := &value.Function{
		Name: DiffElem,
		// The positional is optional -- the a/b form leaves it out -- so it takes
		// none as well, the value Apply fills an unwritten slot with.
		Positional: []value.Param{{Name: "diff", Type: optStr, Default: value.None{}}},
		Named: value.NamedParams{
			nA:       {Name: "a", Type: str},
			nB:       {Name: "b", Type: str},
			nDisplay: {Name: "display", Type: str},
			nLang:    {Name: "lang", Type: str},
		},
		F: inc.diff,
	}

	return map[name.Name]value.Value{
		name.Make(SnippetElem): snippet,
		name.Make(DiffElem):    diff,
	}
}

// includes carries what the two functions need beyond their arguments: the
// filesystem include paths resolve against.
type includes struct {
	fs fs.FS
}

// snippet implements include-snippet: it reads a source file, returning an
// [Include] over a [Highlight].
//
// lines selects a range as "from..to", 1-based and inclusive, with either side
// omittable; display overrides the caption label; lang overrides the lexer
// otherwise chosen from the file name.
func (i *includes) snippet(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	file := string(args[0].(value.Str))
	if file == "" {
		return nil, value.ArgErrorPosf(0, "include-snippet: empty file attribute")
	}

	b, err := fs.ReadFile(i.fs, file)
	if err != nil {
		return nil, value.ArgErrorPosf(0, "include-snippet: %v", err)
	}

	lexer := Lexer{File: file}
	if lang, ok := named.Lookup(nLang); ok {
		lexer = Lexer{Lang: string(lang.(value.Str))}
	}

	// The whole file is highlighted; the range is applied to the result, so one
	// fragment serves every range of the same file.
	from, to := 0, -1
	if lines, ok := named.Lookup(nLines); ok {
		if from, to, err = parseLines(string(lines.(value.Str)), countLines(b)); err != nil {
			return nil, err
		}
	}

	data := &Include{
		File:     file,
		FilePath: file,
		From:     from,
		To:       to,
		Req:      Highlight{Text: b, Lexer: lexer},
	}
	if display, ok := named.Lookup(nDisplay); ok {
		data.File = string(display.(value.Str))
	}
	return &value.Custom{Elem: SnippetElem, Block: true, Value: data}, nil
}

// parseLines reads the range sel names, over a file of n lines, as a start
// index and an exclusive end, where -1 runs to the last line. Both sides of sel
// are 1-based, so a line number below 1 is rejected along with one that is not
// a number at all.
func parseLines(sel string, n int) (from, to int, err error) {
	a, b, _ := strings.Cut(sel, "..")
	from, to = 0, -1
	if a != "" {
		i, err := lineNo(a, sel)
		if err != nil {
			return 0, 0, err
		}
		from = i - 1
	}
	if b != "" {
		if to, err = lineNo(b, sel); err != nil {
			return 0, 0, err
		}
	}
	end := n
	if to >= 0 {
		end = min(to, n)
	}
	if from > end {
		return 0, 0, value.ArgErrorNamedf(nLines, "include-snippet: empty lines range: %q", sel)
	}
	return from, to, nil
}

// lineNo reads s as a line number, one side of the range sel.
func lineNo(s, sel string) (int, error) {
	i, err := strconv.Atoi(s)
	if err != nil || i < 1 {
		return 0, value.ArgErrorNamedf(nLines, "include-snippet: invalid lines attribute: %q", sel)
	}
	return i, nil
}

// countLines returns the number of lines in b, the last one counting whether or
// not it ends in a newline.
func countLines(b []byte) int {
	n := bytes.Count(b, []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// SelectLines cuts lines down to the range an [Include] names.
func (s *Include) SelectLines(lines []highlight.Line) ([]highlight.Line, error) {
	start, end := s.From, len(lines)
	if s.To >= 0 {
		end = min(s.To, end)
	}
	if start > end {
		return nil, fmt.Errorf("include-snippet: empty lines range")
	}
	return lines[start:end], nil
}

// diff implements include-diff: it produces an [Include] over a [ParseDiff] for
// a diff file (the positional) or a [Diff] for two source files (a and b). The
// two forms are mutually exclusive; one of them must be given. Either side of a
// and b may be "/dev/null", standing for an empty file, which is how a pure
// addition or deletion is written.
//
// display and lang mean what they do in [includes.snippet].
func (i *includes) diff(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	file, hasFile := args[0].(value.Str)
	a, hasA := named.Lookup(nA)
	b, hasB := named.Lookup(nB)

	var langName string
	if lang, ok := named.Lookup(nLang); ok {
		langName = string(lang.(value.Str))
	}

	var data *Include
	switch {
	case hasFile && !hasA && !hasB:
		path := string(file)
		raw, err := fs.ReadFile(i.fs, path)
		if err != nil {
			return nil, value.ArgErrorPosf(0, "include-diff: %v", err)
		}
		data = &Include{
			File:     path,
			FilePath: path,
			To:       -1,
			Req:      ParseDiff{Text: raw, Lexer: Lexer{Lang: langName}},
		}

	case !hasFile && hasA && hasB:
		aPath, bPath := string(a.(value.Str)), string(b.(value.Str))
		// a is read before b so that the lexer, when it has to be guessed, is
		// guessed from a fixed one of the two.
		before, err := i.readSide(nA, aPath)
		if err != nil {
			return nil, err
		}
		after, err := i.readSide(nB, bPath)
		if err != nil {
			return nil, err
		}
		lexer := Lexer{Lang: langName}
		if langName == "" {
			guess := aPath
			if guess == devNull {
				guess = bPath
			}
			lexer = Lexer{File: guess}
		}
		data = &Include{
			File:     bPath,
			FilePath: bPath,
			To:       -1,
			Req:      Diff{A: before, B: after, Lexer: lexer},
		}

	default:
		// Blaming the positional is also how a call that wrote none of the
		// three is handled: with nothing at the call site to underline, markst
		// falls back to the call itself.
		return nil, value.ArgErrorPosf(0, "include-diff: either diff or a and b must be specified")
	}

	if display, ok := named.Lookup(nDisplay); ok {
		data.File = string(display.(value.Str))
	}
	return &value.Custom{Elem: DiffElem, Block: true, Value: data}, nil
}

// devNull names the empty file, the side a pure addition or deletion diffs
// against.
const devNull = "/dev/null"

func (i *includes) readSide(arg name.Name, file string) ([]byte, error) {
	if file == devNull {
		return nil, nil
	}
	b, err := fs.ReadFile(i.fs, file)
	if err != nil {
		return nil, value.ArgErrorNamedf(arg, "include-diff: %v", err)
	}
	return b, nil
}
