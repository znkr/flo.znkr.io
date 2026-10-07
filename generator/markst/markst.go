package markst

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst/builtins"
	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Diagnostic is a warning or an error about a document that compiled. A
// document with an error compiles but does not render: see [Doc.Err].
type Diagnostic = markst.Diagnostic

// Severity distinguishes a warning from an error.
type Severity = markst.Severity

const (
	// Warning is the severity of a diagnostic about a document that renders.
	Warning = markst.Warning
	// Error is the severity of a diagnostic about a document that does not.
	Error = markst.Error
)

// OfSeverity returns the diagnostics in diags that have severity s.
func OfSeverity(diags []Diagnostic, s Severity) []Diagnostic {
	var ret []Diagnostic
	for _, d := range diags {
		if d.Severity == s {
			ret = append(ret, d)
		}
	}
	return ret
}

// Lib is a compiled library and the diagnostics compiling it produced. Lib is
// nil if Diags holds an error.
type Lib struct {
	Path  string
	Lib   *markst.Library
	Diags []Diagnostic
}

// Report writes diags to stderr.
//
// Diagnostics are returned rather than printed where they arise, so that they
// can be reported again for a document that did not have to be compiled again.
func Report(diags []Diagnostic) {
	if len(diags) > 0 {
		markst.FormatDiagnostics(os.Stderr, diags)
	}
}

// Doc is a compiled markst document.
type Doc struct {
	// Doc is the document itself and Index the label index built with it. Doc
	// is nil if the document did not compile.
	Doc   *value.Document
	Index *value.Index

	// Meta is what the document says about itself.
	Meta Metadata

	// Summary is the summary the metadata carries, nil if there is none. It is
	// content rather than HTML because rendering it is a step of its own, so
	// that reading a document's title does not wait on it.
	Summary value.Content

	// Diags holds the warnings and errors compiling the document produced. A
	// document with an error does not render: see [Doc.Err].
	Diags []Diagnostic
}

// Metadata is what a document declares in its doc-meta. A document whose
// doc-meta is missing or invalid, or whose type is not one [Load] is given,
// has zero Metadata and an error in [Doc.Diags].
type Metadata struct {
	Title     string
	Type      string
	Published time.Time
	Updated   time.Time
}

// Err returns the errors in d.Diags as one error, or nil if there are none.
func (d *Doc) Err() error {
	errs := OfSeverity(d.Diags, Error)
	if len(errs) == 0 {
		return nil
	}
	return errors.New(Format(errs))
}

// Fragments returns the highlighting the body needs before it can be
// rendered, one artifact per fragment. Declaring them computes nothing. A
// document with errors needs none.
func (d *Doc) Fragments() ([]build.Artifact[builtins.Fragment], error) {
	if d.Doc == nil {
		return nil, nil
	}
	return builtins.Artifacts(d.Doc)
}

// SummaryFragments returns the highlighting the summary needs before it can
// be rendered. They are separate from [Doc.Fragments] so that rendering the
// summary does not wait on the body's highlighting.
func (d *Doc) SummaryFragments() ([]build.Artifact[builtins.Fragment], error) {
	return builtins.Artifacts(d.Summary)
}

// LoadLibrary compiles the markst library in data on its own: a library may
// use nothing another library defines, so that each is an artifact of its own
// file and an edit to one recompiles only that one. Its bindings are then
// available to every document compiled with the returned library, without an
// import: see [Load].
//
// A library that does not compile is returned with its errors in [Lib.Diags].
// The error is reserved for a compile that could not run.
func LoadLibrary(ctx context.Context, path string, data []byte) (*Lib, error) {
	lib, diags, err := markst.CompileLibrary(ctx, path, data)
	if err != nil {
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return &Lib{Path: path, Diags: append(diags, list...)}, nil
	}
	return &Lib{Path: path, Lib: lib, Diags: diags}, nil
}

// Load compiles the markst document in data against libs, whose bindings the
// document can use as if they were built in. Files the document includes are
// read from docFS. types are the types the document's metadata may declare.
//
// Nothing is highlighted: an include is compiled to the request for it, which
// [builtins.Artifacts] declares and the build then runs.
//
// A document that does not compile, uses a library that did not compile, or
// whose metadata is invalid is returned with its errors in [Doc.Diags]. The
// error is reserved for a compile that could not run.
func Load(ctx context.Context, path string, data []byte, docFS fs.FS, libs []*Lib, types []string) (*Doc, error) {
	var index value.Index
	doc := &Doc{Index: &index}

	compiled := make([]*markst.Library, len(libs))
	for i, l := range libs {
		if l.Lib == nil {
			doc.Diags = append(doc.Diags, errorIn(path, "uses a library with errors: "+l.Path))
			continue
		}
		compiled[i] = l.Lib
	}
	if len(doc.Diags) > 0 {
		return doc, nil
	}

	d, diags, err := markst.Compile(ctx, data,
		markst.WithName(path),
		markst.WithLibrary(compiled...),
		markst.WithIndex(&index),
		markst.WithBindings(builtins.Bindings(docFS)),
	)
	if err != nil {
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		doc.Diags = append(diags, list...)
		return doc, nil
	}
	doc.Doc = d
	doc.Diags = diags

	meta, summary, errs := metadata(d)
	if len(errs) == 0 && !slices.Contains(types, meta.Type) {
		errs = append(errs, fmt.Errorf("unknown doc type: %q", meta.Type))
	}
	if len(errs) > 0 {
		for _, err := range errs {
			doc.Diags = append(doc.Diags, errorIn(path, err.Error()))
		}
		return doc, nil
	}
	doc.Meta = meta
	doc.Summary = summary
	return doc, nil
}

// errorIn returns an error diagnostic about the file at path as a whole.
func errorIn(path, msg string) Diagnostic {
	return Diagnostic{Severity: Error, Origin: path, Msg: msg}
}

// Format renders diags the way a compiler does, one per line. Each diagnostic
// names the file it occurred in, so nothing needs to be supplied here. Writing
// to a [strings.Builder] cannot fail, so the error from FormatDiagnostics
// carries no information either.
func Format(diags []Diagnostic) string {
	var sb strings.Builder
	markst.FormatDiagnostics(&sb, diags)
	return strings.TrimSuffix(sb.String(), "\n")
}

func metadata(doc *value.Document) (Metadata, value.Content, []error) {
	v, ok := markst.Query(doc, name.Make("doc-meta"))
	if !ok {
		return Metadata{}, nil, []error{errors.New("missing doc-meta")}
	}
	d, err := dictFromValue(v)
	if err != nil {
		return Metadata{}, nil, []error{errors.New("doc-meta is not a dict")}
	}

	var meta Metadata
	if title, ok := d.get[value.Str]("title"); ok {
		meta.Title = string(title)
	}
	if typ, ok := d.get[value.Str]("type"); ok {
		meta.Type = string(typ)
	}
	if published, ok := d.get[value.Datetime]("published"); ok {
		meta.Published = published.T
		meta.Updated = published.T
	}
	if updated, ok := d.get[value.Datetime]("updated"); ok {
		meta.Updated = updated.T
	}
	summary, _ := d.get[value.Content]("summary")
	return meta, summary, d.errors
}

type dict struct {
	d      *value.Dict
	errors []error
}

func dictFromValue(v value.Value) (*dict, error) {
	d, ok := v.(*value.Dict)
	if !ok {
		return nil, fmt.Errorf("not a dict")
	}
	return &dict{d: d}, nil
}

func (d *dict) get[T value.Value](key string) (T, bool) {
	v, ok := d.d.Elems.Get(value.Str(key))
	if !ok {
		var zero T
		return zero, false
	}
	t, ok := v.(T)
	if !ok {
		if _, ok := v.(value.None); !ok {
			d.errors = append(d.errors, fmt.Errorf("value for key %s has wrong type %T, expected %T", key, v, *new(T)))
		}
		var zero T
		return zero, false
	}
	return t, true
}
