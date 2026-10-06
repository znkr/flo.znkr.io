package renderers

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"path"
	"slices"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/highlight"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/site"
)

// RenderPage wraps body and toc in the page template meta.Type names, which
// can load assets.
func RenderPage(templates *template.Template, meta site.Metadata, assets Assets, body, toc []byte) ([]byte, error) {
	// Lookup alone is not enough: an empty name finds the root template, and a
	// fragment name finds a template that is no page.
	page := templates.Lookup(meta.Type)
	if !slices.Contains(site.DocTypes, meta.Type) || page == nil {
		return nil, fmt.Errorf("unknown doc type: %s", meta.Type)
	}

	head, err := NewHead(meta, assets)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = page.Execute(&buf, struct {
		Meta     site.Metadata
		Head     Head
		Profiles []Profile
		Content  template.HTML
		TOC      template.HTML
	}{
		Meta:     meta,
		Head:     head,
		Profiles: authorProfiles,
		Content:  template.HTML(body),
		TOC:      template.HTML(toc),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}

// RenderBody renders doc's body, drawing its includes with the fragment
// templates. frags are the document's fragments, keyed the way
// [builtins.Artifact] keys them; docRoot is the directory the document was read
// from, relative to the site root, which the include captions link into; fn
// says how the footnotes are drawn.
func RenderBody(templates *template.Template, doc *markst.Doc, frags map[build.Key]builtins.Fragment, docPath, docRoot string, fn markst.Footnotes) ([]byte, error) {
	inc, err := newIncludes(templates, docRoot)
	if err != nil {
		return nil, err
	}
	return markst.RenderBody(doc, frags, docPath, inc, fn)
}

// includes draws the include elements with fragments/include_snippet and
// fragments/include_diff. It implements [markst.Includes].
type includes struct {
	snippet, diff *template.Template
	docRoot       string
}

func newIncludes(templates *template.Template, docRoot string) (includes, error) {
	snippet := templates.Lookup("fragments/include_snippet")
	if snippet == nil {
		return includes{}, fmt.Errorf("template not found fragments/include_snippet")
	}
	diff := templates.Lookup("fragments/include_diff")
	if diff == nil {
		return includes{}, fmt.Errorf("template not found fragments/include_diff")
	}
	return includes{snippet: snippet, diff: diff, docRoot: docRoot}, nil
}

func (i includes) Snippet(w io.Writer, inc *builtins.Include, lines builtins.Lines) error {
	return execute(w, i.snippet, struct {
		File, FilePath string
		Lines          []highlight.Line
	}{
		File:     inc.File,
		FilePath: path.Join(i.docRoot, inc.FilePath),
		Lines:    lines,
	})
}

func (i includes) Diff(w io.Writer, inc *builtins.Include, edits builtins.Edits) error {
	return execute(w, i.diff, struct {
		File, FilePath string
		Diff           []highlight.Edit
	}{
		File:     inc.File,
		FilePath: path.Join(i.docRoot, inc.FilePath),
		Diff:     edits,
	})
}

// execute renders one fragment into the document.
func execute(w io.Writer, t *template.Template, data any) error {
	if err := t.Execute(w, data); err != nil {
		return fmt.Errorf("rendering %s: %v", t.Name(), err)
	}
	return nil
}
