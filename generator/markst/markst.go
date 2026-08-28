package markst

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/markst/html"
	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Library is a compiled set of markst bindings, shared by every document on
// the site. It is an alias so that callers of [LoadLibrary] and [Load] need
// only this package.
type Library = markst.Library

// LoadLibrary compiles the markst library in data, which may use anything the
// libraries in deps define. Its own bindings are then available to every
// document compiled with the returned library, without an import: see [Load].
func LoadLibrary(path string, data []byte, deps []*Library) (*markst.Library, error) {
	lib, diags, err := markst.CompileLibrary(path, data, markst.WithLibrary(deps...))
	if len(diags) > 0 {
		markst.FormatDiagnostics(os.Stderr, diags)
	}
	if err != nil {
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, fmt.Errorf("%s: %v", path, err)
		}
		return nil, errors.New(format(list))
	}
	return lib, nil
}

// Load compiles the markst document in data against libs, whose bindings the
// document can use as if they were built in. Diagnostics are reported with the
// file and position they occurred at, in the form editors understand — which
// for a failure inside a library function is the library's file, plus the call
// site in the document that reached it. Warnings go to stderr, errors are
// returned.
//
// srcDir is the directory the document was read from and sitePath its path on
// the site. Both go to the include functions, which resolve the files they
// pull in against the one and link their captions relative to the other; see
// [builtins.Bindings] for why those cannot come from a library.
func Load(path, srcDir, sitePath string, data []byte, libs []*Library) (*site.Metadata, *html.RenderData, error) {
	d, diags, err := markst.Compile(data,
		markst.WithName(path),
		markst.WithLibrary(libs...),
		markst.WithBindings(builtins.Bindings(srcDir, sitePath)),
	)
	if len(diags) > 0 {
		// Warnings describe a document that compiled, so they are reported
		// but don't stop the load. Straight to stderr, in compiler form,
		// rather than through log: a diagnostic can span multiple lines and
		// a log prefix on the first of them only makes it harder to read.
		markst.FormatDiagnostics(os.Stderr, diags)
	}
	if err != nil {
		var list markst.DiagnosticList
		if !errors.As(err, &list) {
			return nil, nil, fmt.Errorf("%s: %v", path, err)
		}
		return nil, nil, errors.New(format(list))
	}
	meta, err := metadata(d)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", path, err)
	}
	return meta, &html.RenderData{
		Doc: d,
	}, nil
}

// format renders diags the way a compiler does, one per line. Each diagnostic
// names the file it occurred in, so nothing needs to be supplied here. Writing
// to a [strings.Builder] cannot fail, so the error from FormatDiagnostics
// carries no information either.
func format(diags []markst.Diagnostic) string {
	var sb strings.Builder
	markst.FormatDiagnostics(&sb, diags)
	return strings.TrimSuffix(sb.String(), "\n")
}

func metadata(doc *value.Document) (*site.Metadata, error) {
	v, ok := markst.Query(doc, name.Make("doc-meta"))
	if !ok {
		return nil, fmt.Errorf("missing doc-meta in markst document: %v", value.FormatContent(doc))
	}
	d, err := dictFromValue(v)
	if err != nil {
		return nil, fmt.Errorf("doc-meta is not a dict")
	}

	meta := new(site.Metadata)
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
	if summary, ok := d.get[value.Content]("summary"); ok {
		s, err := html.RenderSummary(summary)
		if err != nil {
			return nil, fmt.Errorf("rendering summary: %v", err)
		}
		meta.Abstract = s
	}
	return meta, errors.Join(d.errors...)
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
