package builtins

import (
	"fmt"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/highlight"
	"znkr.io/markst/value"
)

// Lexer says how a fragment is highlighted: by an explicit language name, or by
// the file name one is guessed from. At most one is set.
//
// It is the two strings the document wrote rather than the lexer they resolve
// to, because matching a file name against chroma's lexers costs milliseconds
// and so belongs with the highlighting rather than with the compile.
type Lexer struct {
	Lang string
	File string
}

func (l Lexer) option() highlight.Option {
	switch {
	case l.Lang != "":
		return highlight.Lang(l.Lang)
	case l.File != "":
		return highlight.LangFromFilename(l.File)
	default:
		return nil
	}
}

// Request is one piece of highlighting a document needs before it can be
// rendered.
//
// A request holds what [Request.Build] reads and nothing else, so that the
// build can key a fragment on the request alone: a field Build ignores splits
// two fragments that are the same. What is drawn around a fragment goes in the
// [Include] the request belongs to.
type Request interface {
	Build() (Fragment, error)
}

// Highlight highlights Text. It is what a raw span and an include-snippet both
// need, so the two share their fragments.
type Highlight struct {
	Text  []byte
	Lexer Lexer
}

// Diff diffs A against B.
type Diff struct {
	A, B  []byte
	Lexer Lexer
}

// ParseDiff reads Text as a unified diff.
type ParseDiff struct {
	Text  []byte
	Lexer Lexer
}

// Fragment is what a [Request] builds: a closed set, so that the presenter can
// tell what it was given.
type Fragment interface {
	fragment()
}

// Lines is what a [Highlight] builds.
type Lines []highlight.Line

// Edits is what a [Diff] or a [ParseDiff] builds.
type Edits []highlight.Edit

func (Lines) fragment() {}
func (Edits) fragment() {}

func (r Highlight) Build() (Fragment, error) {
	lines, err := highlight.Highlight(string(r.Text), r.Lexer.option())
	return Lines(lines), err
}

func (r Diff) Build() (Fragment, error) {
	edits, err := highlight.Diff(string(r.A), string(r.B), r.Lexer.option())
	return Edits(edits), err
}

func (r ParseDiff) Build() (Fragment, error) {
	edits, err := highlight.ParseDiff(string(r.Text), r.Lexer.option())
	return Edits(edits), err
}

// Artifact returns the artifact holding the highlighting node needs.
//
// Declaring one computes nothing, so the walk that plans a document's fragments
// and the presenter that draws them can each ask for the same node and get the
// same artifact. That is how a fragment finds the element it belongs to: by
// key, not by the position it came in.
func Artifact(node value.Content) (build.Artifact[Fragment], error) {
	r, err := request(node)
	if err != nil {
		return build.Artifact[Fragment]{}, err
	}
	return build.Derive[Fragment]("fragment", Request.Build, r), nil
}

// Artifacts returns the highlighting that has to be done before c can be
// rendered.
//
// A document's metadata is not walked into, so the fragments of a document and
// those of the summary it carries are separate: rendering the summary does not
// wait on the body's highlighting.
func Artifacts(c value.Content) ([]build.Artifact[Fragment], error) {
	if c == nil {
		return nil, nil
	}
	var arts []build.Artifact[Fragment]
	for cur := range value.Preorder(c, value.SetOf(value.KindCustom, value.KindRaw)) {
		a, err := Artifact(cur.Node())
		if err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	return arts, nil
}

// request returns what has to be highlighted for node.
func request(node value.Content) (Request, error) {
	switch n := node.(type) {
	case *value.Raw:
		// A raw span is markst's own node, with nowhere to carry an [Include],
		// so its request is built here.
		return Highlight{Text: []byte(n.Text), Lexer: Lexer{Lang: n.Lang}}, nil
	case *value.Custom:
		inc, ok := n.Value.(*Include)
		if !ok {
			return nil, fmt.Errorf("unsupported custom payload: %T", n.Value)
		}
		return inc.Req, nil
	default:
		return nil, fmt.Errorf("unsupported fragment element: %T", node)
	}
}
