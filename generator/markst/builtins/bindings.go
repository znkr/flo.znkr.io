// Package builtins holds the markst functions the generator provides to every
// document, as opposed to the ones written in markst under lib.
package builtins

import (
	"io/fs"
	"strconv"
	"strings"

	"flo.znkr.io/generator/highlight"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Element names the markst elements this package produces. The presenter
// switches on the payload type rather than on the name, so these are only what
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

// SnippetData is what fragments/include_snippet renders.
type SnippetData struct {
	// File is the caption's label — the display name, which defaults to the
	// path the snippet was read from.
	File string

	// FilePath is the caption link's target, relative to the site root.
	FilePath string

	Lines []highlight.Line
}

// DiffData is what fragments/include_diff renders.
type DiffData struct {
	File     string
	FilePath string
	Diff     []highlight.Edit
}

// Bindings returns the markst bindings a document gets, resolving paths
// against srcDir and linking captions relative to sitePath.
//
// Both are per-document, which is why these are built per compile rather than
// shared through a markst library: a library is compiled once for the whole
// site and cannot know which document is calling it.
//
// The functions do their work while the document is compiling — reading the
// file, computing the diff, running the highlighter — and hand the finished
// table to the presenter inside a [value.Custom]. That is what makes a missing
// file or a malformed line range a compile error pointing at the argument that
// caused it, rather than a failure much later with nothing to point at.
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
		// The positional is optional — the a/b form leaves it out — so it takes
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

// includes carries what the two functions need beyond their arguments: srcDir
// is the directory the including document was read from, which every path is
// resolved against, and sitePath is that document's path on the site, which the
// caption links relative to.
type includes struct {
	fs fs.FS
}

// snippet implements include-snippet: it reads a source file and highlights it,
// returning a [SnippetData].
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

	lopt := highlight.LangFromFilename(file)
	if lang, ok := named.Lookup(nLang); ok {
		lopt = highlight.Lang(string(lang.(value.Str)))
	}
	hl, err := highlight.Highlight(string(b), lopt)
	if err != nil {
		return nil, value.ArgErrorNamedf(nLang, "include-snippet: %v", err)
	}

	if lines, ok := named.Lookup(nLines); ok {
		hl, err = selectLines(hl, string(lines.(value.Str)))
		if err != nil {
			return nil, err
		}
	}

	data := &SnippetData{
		File:     file,
		FilePath: file,
		Lines:    hl,
	}
	if display, ok := named.Lookup(nDisplay); ok {
		data.File = string(display.(value.Str))
	}
	return &value.Custom{Elem: SnippetElem, Block: true, Value: data}, nil
}

// selectLines cuts lines down to the range sel names.
func selectLines(lines []highlight.Line, sel string) ([]highlight.Line, error) {
	from, to, _ := strings.Cut(sel, "..")
	start, end := 0, len(lines)
	if from != "" {
		i, err := strconv.Atoi(from)
		if err != nil {
			return nil, value.ArgErrorNamedf(nLines, "include-snippet: invalid lines attribute: %q", sel)
		}
		start = max(start, i-1)
	}
	if to != "" {
		i, err := strconv.Atoi(to)
		if err != nil {
			return nil, value.ArgErrorNamedf(nLines, "include-snippet: invalid lines attribute: %q", sel)
		}
		end = min(i, end)
	}
	if start > end {
		return nil, value.ArgErrorNamedf(nLines, "include-snippet: empty lines range: %q", sel)
	}
	return lines[start:end], nil
}

// diff implements include-diff: it produces a [DiffData], either by reading a
// diff file (the positional) or by diffing two source files (a and b). The two
// forms are mutually exclusive; one of them must be given. Either side of a and
// b may be "/dev/null", standing for an empty file, which is how a pure
// addition or deletion is written.
//
// display and lang mean what they do in [includes.snippet].
func (i *includes) diff(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	file, hasFile := args[0].(value.Str)
	a, hasA := named.Lookup(nA)
	b, hasB := named.Lookup(nB)

	var lopt highlight.Option
	if lang, ok := named.Lookup(nLang); ok {
		lopt = highlight.Lang(string(lang.(value.Str)))
	}

	var data *DiffData
	switch {
	case hasFile && !hasA && !hasB:
		path := string(file)
		raw, err := fs.ReadFile(i.fs, path)
		if err != nil {
			return nil, value.ArgErrorPosf(0, "include-diff: %v", err)
		}
		edits, err := highlight.ParseDiff(string(raw), lopt)
		if err != nil {
			return nil, value.ArgErrorPosf(0, "include-diff: %v", err)
		}
		data = &DiffData{
			File:     path,
			FilePath: path,
			Diff:     edits,
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
		if lopt == nil {
			guess := aPath
			if guess == devNull {
				guess = bPath
			}
			lopt = highlight.LangFromFilename(guess)
		}
		edits, err := highlight.Diff(string(before), string(after), lopt)
		if err != nil {
			return nil, value.ArgErrorNamedf(nLang, "include-diff: %v", err)
		}
		data = &DiffData{
			File:     bPath,
			FilePath: bPath,
			Diff:     edits,
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
