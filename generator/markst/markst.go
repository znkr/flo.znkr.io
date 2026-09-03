package markst

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/site"
	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Diagnostic is a warning about a document that compiled anyway.
type Diagnostic = markst.Diagnostic

// Lib is a compiled library and the diagnostics compiling it produced.
type Lib struct {
	Lib   *markst.Library
	Diags []Diagnostic
}

// libraries returns the compiled libraries in libs.
func libraries(libs []*Lib) []*markst.Library {
	ret := make([]*markst.Library, len(libs))
	for i, l := range libs {
		ret[i] = l.Lib
	}
	return ret
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
	// Doc is the document itself and Index the label index built with it.
	Doc   *value.Document
	Index *value.Index

	// Meta is what the document says about itself. Its Summary is empty:
	// rendering the summary is a step of its own, so that reading a document's
	// title does not wait on it. See [Doc.Summary].
	Meta site.Metadata

	// Summary is the summary the metadata carries, nil if there is none. It is
	// content rather than HTML for the same reason.
	Summary value.Content

	Diags []Diagnostic
}

// LoadLibrary compiles the markst library in data, which may use anything the
// libraries in deps define. Its own bindings are then available to every
// document compiled with the returned library, without an import: see [Load].
//
// Diagnostics are returned in [Lib.Diags] on success and written to stderr on
// failure, where there is no compiled library for a caller to hold them with.
func LoadLibrary(ctx context.Context, path string, data []byte, deps []*Lib) (*Lib, error) {
	lib, diags, err := markst.CompileLibrary(ctx, path, data, markst.WithLibrary(libraries(deps)...))
	if err != nil {
		Report(diags)
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return nil, errors.New(Format(list))
	}
	return &Lib{Lib: lib, Diags: diags}, nil
}

// Load compiles the markst document in data against libs, whose bindings the
// document can use as if they were built in. Files the document includes are
// read from docFS.
//
// Nothing is highlighted: an include is compiled to the request for it, which
// [builtins.Artifacts] declares and the build then runs.
//
// Diagnostics are returned in [Doc.Diags] on success and written to stderr on
// failure, where there is no document for a caller to hold them with.
func Load(ctx context.Context, path string, data []byte, docFS fs.FS, libs []*Lib) (*Doc, error) {
	var index value.Index
	d, diags, err := markst.Compile(ctx, data,
		markst.WithName(path),
		markst.WithLibrary(libraries(libs)...),
		markst.WithIndex(&index),
		markst.WithBindings(builtins.Bindings(docFS)),
	)
	if err != nil {
		Report(diags)
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return nil, errors.New(Format(list))
	}
	meta, summary, err := metadata(d)
	if err != nil {
		Report(diags)
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Doc{
		Doc:     d,
		Index:   &index,
		Meta:    meta,
		Summary: summary,
		Diags:   diags,
	}, nil
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

func metadata(doc *value.Document) (site.Metadata, value.Content, error) {
	v, ok := markst.Query(doc, name.Make("doc-meta"))
	if !ok {
		return site.Metadata{}, nil, fmt.Errorf("missing doc-meta in markst document: %v", value.FormatContent(doc))
	}
	d, err := dictFromValue(v)
	if err != nil {
		return site.Metadata{}, nil, fmt.Errorf("doc-meta is not a dict")
	}

	var meta site.Metadata
	if title, ok := d.get[value.Str]("title"); ok {
		meta.Title = string(title)
	}
	if typ, ok := d.get[value.Str]("type"); ok {
		meta.Type = string(typ)
	}
	if !slices.Contains(site.DocTypes, meta.Type) {
		d.errors = append(d.errors, fmt.Errorf("unknown doc type: %q", meta.Type))
	}
	if published, ok := d.get[value.Datetime]("published"); ok {
		meta.Published = published.T
		meta.Updated = published.T
	}
	if updated, ok := d.get[value.Datetime]("updated"); ok {
		meta.Updated = updated.T
	}
	summary, _ := d.get[value.Content]("summary")
	return meta, summary, errors.Join(d.errors...)
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
