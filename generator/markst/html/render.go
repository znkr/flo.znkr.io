// Package html renders a compiled markst document as the HTML the site serves.
//
// The rendering itself is znkr.io/markst/html's job. What is here is the part
// that belongs to this site: the fragment templates the include elements are
// drawn with, the anchor link on every heading, the site-relative resolution of
// image paths, and the table of contents the article template puts beside the
// body.
//
// Highlighting is not done here. A document's fragments are computed before it
// is rendered and passed in, which is what lets an unchanged snippet survive an
// edit to the prose around it.
package html

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"path"
	"slices"
	"strconv"
	"strings"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/highlight"
	gmarkst "flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/site"
	"znkr.io/markst"
	mhtml "znkr.io/markst/html"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// RenderPage renders doc as a whole page, wrapped in the template its metadata
// asks for. frags are the document's fragments, keyed the way
// [builtins.Artifact] keys them.
func RenderPage(templates *template.Template, doc *gmarkst.Doc, meta site.Metadata, frags map[build.Key]builtins.Fragment, docPath, docRoot string) ([]byte, error) {
	// Lookup alone is not enough: an empty name finds the root template, and a
	// fragment name finds a template that is no page.
	page := templates.Lookup(meta.Type)
	if !slices.Contains(site.DocTypes, meta.Type) || page == nil {
		return nil, fmt.Errorf("%s: unknown doc type: %s", docPath, meta.Type)
	}

	r, err := newRenderer(templates, doc, frags, docPath, docRoot)
	if err != nil {
		return nil, err
	}
	content, err := r.content(r.renderWithFootnotes)
	if err != nil {
		return nil, err
	}
	toc, err := r.renderTOC()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = page.Execute(&buf, struct {
		Meta    site.Metadata
		Content template.HTML
		TOC     template.HTML
	}{
		Meta:    meta,
		Content: template.HTML(content),
		TOC:     template.HTML(toc),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}

// RenderContent renders doc's body alone, without the page around it. It is
// what the feed embeds, so its footnotes are rendered the plain way: a feed
// reader has neither the stylesheet nor the script the panels need.
func RenderContent(templates *template.Template, doc *gmarkst.Doc, frags map[build.Key]builtins.Fragment, docPath, docRoot string) ([]byte, error) {
	r, err := newRenderer(templates, doc, frags, docPath, docRoot)
	if err != nil {
		return nil, err
	}
	return r.content(r.render)
}

// RenderSummary renders the summary doc carries in its metadata, or "" if it
// carries none. It is a fragment of a document rather than one of its own, so
// it is rendered against the site root and without the endnote list a whole
// page gets.
func RenderSummary(doc *gmarkst.Doc, frags map[build.Key]builtins.Fragment) (string, error) {
	if doc.Summary == nil {
		return "", nil
	}
	r := &renderer{path: "/", frags: frags}
	var buf strings.Builder
	err := mhtml.Render(&buf, doc.Summary, append(r.options(r.render), mhtml.WithoutFootnoteList())...)
	return buf.String(), err
}

// renderer holds what the site brings to a render: where the document sits, so
// that the paths in it resolve, the fragments its includes and raw spans were
// highlighted to, and the templates those fragments are drawn with.
type renderer struct {
	path          string
	docRoot       string
	snippet, diff *template.Template
	doc           *value.Document
	index         *value.Index
	frags         map[build.Key]builtins.Fragment
	notes         notes
	cited         []value.Content
}

// notes numbers a document's footnotes, looked up by the footnote itself and by
// the label an @ref cites it with.
type notes struct {
	byNote  map[*value.Footnote]int
	byLabel map[name.Name]*value.Footnote
}

func indexNotes(d *value.Document) notes {
	ns := notes{
		byNote:  make(map[*value.Footnote]int),
		byLabel: make(map[name.Name]*value.Footnote),
	}
	for i, fn := range mhtml.Footnotes(d) {
		ns.byNote[fn] = i + 1
		if l := fn.GetLabel(); l != nil {
			ns.byLabel[l.Name] = fn
		}
	}
	return ns
}

func newRenderer(templates *template.Template, doc *gmarkst.Doc, frags map[build.Key]builtins.Fragment, docPath, docRoot string) (*renderer, error) {
	snippet := templates.Lookup("fragments/include_snippet")
	if snippet == nil {
		return nil, fmt.Errorf("template not found fragments/include_snippet")
	}
	diff := templates.Lookup("fragments/include_diff")
	if diff == nil {
		return nil, fmt.Errorf("template not found fragments/include_diff")
	}
	return &renderer{
		path:    docPath,
		docRoot: docRoot,
		snippet: snippet,
		diff:    diff,
		doc:     doc.Doc,
		index:   doc.Index,
		frags:   frags,
		notes:   indexNotes(doc.Doc),
	}, nil
}

func (r *renderer) content(el mhtml.Element) ([]byte, error) {
	var buf bytes.Buffer
	if err := mhtml.Render(&buf, r.doc, r.options(el)...); err != nil {
		return nil, err
	}
	if err := r.writeNotes(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// options is how the site's rendering differs from the default one. el is the
// element hook: renderWithFootnotes for a render that lands on a page of this
// site, tocEntry for the table of contents, render for one that lands anywhere
// else.
func (r *renderer) options(el mhtml.Element) []mhtml.Option {
	return []mhtml.Option{
		// <h1> is the article title, written by the page template.
		mhtml.WithHeadingLevel(2),
		mhtml.WithImageURL(func(p string) (string, error) { return path.Join(r.path, p), nil }),
		mhtml.WithIndex(r.index),
		mhtml.WithElement(el),
	}
}

// renderWithFootnotes renders a footnote where it is cited, as a mark that
// opens the note, and everything else the way render does. The note itself is
// written by writeNotes, after the document body.
func (r *renderer) renderWithFootnotes(e *mhtml.Encoder, c value.Content) (bool, error) {
	switch c := c.(type) {
	case *value.Footnote:
		n, ok := r.notes.byNote[c]
		if !ok {
			// A footnote outside the document the notes were indexed from,
			// such as one in a summary. It has no note to open.
			return false, nil
		}
		r.citation(e, n, "fnref:"+strconv.Itoa(n), c.Body)
		return true, nil

	case *value.Ref:
		fn, ok := r.notes.byLabel[c.Target]
		if !ok {
			return false, nil // a reference to something that is no footnote
		}
		r.citation(e, r.notes.byNote[fn], "", fn.Body)
		return true, nil
	}
	return r.render(e, c)
}

// citation writes the mark a footnote is cited with: its number, as a button
// that opens the note. id is the anchor the endnote links back to, empty for an
// @ref citation, because the return trip goes to where the footnote was written
// rather than to every mention.
//
// Every citation gets a note of its own, so that the anchor name a note is
// positioned against names exactly one mark.
func (r *renderer) citation(e *mhtml.Encoder, n int, id string, body value.Content) {
	i := len(r.cited)
	r.cited = append(r.cited, body)
	num := strconv.Itoa(n)

	var attrs []mhtml.Attr
	if id != "" {
		attrs = append(attrs, mhtml.Attr{Name: "id", Value: id})
	}
	attrs = append(attrs, mhtml.Attr{Name: "style", Value: "anchor-name:" + anchorName(i)})
	e.Start("sup", attrs...)
	e.Start("button",
		mhtml.Attr{Name: "type", Value: "button"},
		mhtml.Attr{Name: "class", Value: "footnote-mark"},
		mhtml.Attr{Name: "popovertarget", Value: noteID(i)},
		mhtml.Attr{Name: "aria-label", Value: "Footnote " + num},
	)
	e.Text(num)
	e.End("button")
	e.End("sup")
}

// writeNotes writes the note each citation opens. They come after the document
// body because a note holds blocks and a citation sits inside a paragraph.
// Nothing is written for a render that cited no footnotes.
//
// A note is a second rendering of a body the endnote list has already written,
// so it carries no ids: the anchors an @ref points at are the endnote's.
func (r *renderer) writeNotes(buf *bytes.Buffer) error {
	opts := append(r.options(r.render), mhtml.WithoutFootnoteList(), mhtml.WithoutLabelIDs())
	for i, body := range r.cited {
		fmt.Fprintf(buf, `<div id="%s" class="footnote-body" popover style="position-anchor:%s">`, noteID(i), anchorName(i))
		if err := mhtml.Render(buf, body, opts...); err != nil {
			return err
		}
		buf.WriteString("</div>\n")
	}
	return nil
}

// noteID returns the id of the note the i-th citation opens.
func noteID(i int) string { return "footnote-body-" + strconv.Itoa(i+1) }

// anchorName returns the name that note is positioned against.
func anchorName(i int) string { return "--footnote-" + strconv.Itoa(i+1) }

func (r *renderer) render(e *mhtml.Encoder, c value.Content) (bool, error) {
	switch c := c.(type) {
	case *value.Heading:
		// Every heading gets a link to itself, which the stylesheet shows on
		// hover. The label is what it points at; realization guarantees one.
		tag := fmt.Sprintf("h%d", min(c.Depth+1, 6))
		e.Start(tag, mhtml.Attr{Name: "id", Value: label(c)})
		e.Content(c.Body)
		e.Start("a", mhtml.Attr{Name: "href", Value: "#" + label(c)}, mhtml.Attr{Name: "class", Value: "anchor-link"})
		e.End("a")
		e.End(tag)
		e.Newline()
		return true, nil

	case *value.Raw:
		code, err := r.rawCode(c)
		if err != nil {
			return true, err
		}
		if c.Block {
			e.HTML("<pre><code>" + code + "</code></pre>")
			e.Newline()
		} else {
			e.HTML("<code>" + code + "</code>")
		}
		return true, nil

	case *value.Custom:
		// An element the generator put in the document while it compiled; see
		// builtins.Bindings. The include says what to draw around the fragment
		// that was computed for it, and the fragment says which template draws
		// it.
		p, ok := c.Value.(*builtins.Include)
		if !ok {
			return true, fmt.Errorf("unsupported custom payload: %T", c.Value)
		}
		frag, err := r.fragment(c, c.Elem)
		if err != nil {
			return true, err
		}
		switch f := frag.(type) {
		case builtins.Lines:
			lines, err := p.SelectLines(f)
			if err != nil {
				return true, err
			}
			return true, r.execute(e, r.snippet, struct {
				File, FilePath string
				Lines          []highlight.Line
			}{
				File:     p.File,
				FilePath: path.Join(r.docRoot, p.FilePath),
				Lines:    lines,
			})
		case builtins.Edits:
			return true, r.execute(e, r.diff, struct {
				File, FilePath string
				Diff           []highlight.Edit
			}{
				File:     p.File,
				FilePath: path.Join(r.docRoot, p.FilePath),
				Diff:     f,
			})
		default:
			return true, fmt.Errorf("%s resolved to a %T", c.Elem, f)
		}
	}
	return false, nil
}

func label(c value.Content) string {
	l := c.GetLabel()
	if l == nil {
		return ""
	}
	return l.Name.String()
}

// execute renders one fragment into the document.
func (r *renderer) execute(e *mhtml.Encoder, t *template.Template, data any) error {
	if t == nil {
		return fmt.Errorf("%T cannot be rendered here: no templates", data)
	}
	if err := t.Execute(e, data); err != nil {
		return fmt.Errorf("rendering %s: %v", t.Name(), err)
	}
	return nil
}

// fragment returns the highlighting computed for node. what names the node in
// the error when the build did not compute it.
func (r *renderer) fragment(node value.Content, what string) (builtins.Fragment, error) {
	a, err := builtins.Artifact(node)
	if err != nil {
		return nil, err
	}
	frag, ok := r.frags[a.Key()]
	if !ok {
		return nil, fmt.Errorf("no fragment for %s", what)
	}
	return frag, nil
}

// rawCode returns the highlighted form of a raw node.
func (r *renderer) rawCode(v *value.Raw) (string, error) {
	frag, err := r.fragment(v, "raw "+rawKind(v))
	if err != nil {
		return "", err
	}
	lines, ok := frag.(builtins.Lines)
	if !ok {
		return "", fmt.Errorf("raw %s resolved to a %T, want highlighted lines", rawKind(v), frag)
	}

	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(string(line.Content))
	}
	code := sb.String()
	if !strings.HasSuffix(v.Text, "\n") {
		// Lexers append a newline to input that doesn't end in one; the node's
		// text is what has to come out the other end, nothing more.
		code = strings.TrimSuffix(code, "\n")
	}
	return code, nil
}

func rawKind(v *value.Raw) string {
	if v.Block {
		return "block"
	}
	return "span"
}

// tocEntry renders a heading body for the table of contents, and everything in
// it the way render does. A footnote cited in the heading gets no mark, whether
// it was written there or reached with an @ref: the entry is a link, and a mark
// inside it is a link inside a link.
func (r *renderer) tocEntry(e *mhtml.Encoder, c value.Content) (bool, error) {
	switch c := c.(type) {
	case *value.Footnote:
		return true, nil
	case *value.Ref:
		if _, ok := r.notes.byLabel[c.Target]; ok {
			return true, nil
		}
	}
	return r.render(e, c)
}

// renderTOC renders the document's headings as a nested <ul> list, each entry
// linking to its heading. It fails if a heading has no label, and with it no
// anchor to link to.
func (r *renderer) renderTOC() ([]byte, error) {
	var buf bytes.Buffer
	// A heading body is a fragment of the document: the endnotes belong to the
	// page, and the marks are dropped by tocEntry.
	opts := append(r.options(r.tocEntry), mhtml.WithoutFootnoteList())
	if err := r.writeTOC(&buf, markst.Outline(r.index), opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *renderer) writeTOC(buf *bytes.Buffer, sections []*markst.Section, opts []mhtml.Option) error {
	if len(sections) == 0 {
		return nil
	}
	buf.WriteString("<ul>")
	for _, s := range sections {
		if s.Heading.GetLabel() == nil {
			return fmt.Errorf("heading has no label: %s", value.FormatContent(s.Heading.Body))
		}
		buf.WriteString("<li>")
		fmt.Fprintf(buf, "<a href=\"#%s\">", html.EscapeString(label(s.Heading)))
		if err := mhtml.Render(buf, s.Heading.Body, opts...); err != nil {
			return err
		}
		buf.WriteString("</a>")
		if err := r.writeTOC(buf, s.Children, opts); err != nil {
			return err
		}
		buf.WriteString("</li>\n")
	}
	buf.WriteString("</ul>")
	return nil
}
