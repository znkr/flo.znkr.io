// Package markst compiles markst documents for this site and renders them as
// HTML.
//
// Compiling is znkr.io/markst's job, rendering znkr.io/markst/html's. What is
// here is the part that belongs to this site: the libraries every document gets
// without an import, the metadata a document declares, the include elements
// and the highlighting they defer to the build, the anchor link on every
// heading, the site-relative resolution of image paths, the marks and notes
// footnotes are drawn as, and the table of contents. Nothing here executes a
// template: the includes are drawn through [Includes], and the page around a
// body is the caller's.
package markst

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"path"
	"strconv"
	"strings"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst/builtins"
	"znkr.io/markst"
	mhtml "znkr.io/markst/html"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Highlighting is not done here. A document's fragments are computed before it
// is rendered and passed in, which is what lets an unchanged snippet survive an
// edit to the prose around it.

// Includes draws the include elements of a document. lines and edits are the
// highlighting computed for inc, lines already cut to the range it selects.
type Includes interface {
	Snippet(w io.Writer, inc *builtins.Include, lines builtins.Lines) error
	Diff(w io.Writer, inc *builtins.Include, edits builtins.Edits) error
}

// Footnotes says how a body's footnotes are drawn.
type Footnotes int

const (
	// Endnotes cites a footnote as a link to the list after the body. It
	// needs no stylesheet or script.
	Endnotes Footnotes = iota
	// Popovers also draws each citation as a mark that opens the note in
	// place. The page's stylesheet and script make the mark work.
	Popovers
)

// RenderBody renders doc's body, without a page around it. frags are the
// document's fragments, keyed the way [builtins.Artifact] keys them, inc draws
// its includes, and fn says how its footnotes are drawn.
func RenderBody(doc *Doc, frags map[build.Key]builtins.Fragment, docPath string, inc Includes, fn Footnotes) ([]byte, error) {
	r := newRenderer(doc, frags, docPath, inc)
	if fn == Popovers {
		return r.content(r.renderWithFootnotes)
	}
	return r.content(r.render)
}

// RenderTOC renders doc's headings as a nested list, each entry linking to its
// heading. It fails if a heading has no label, and with it no anchor to link
// to. It returns nothing for a document without headings.
func RenderTOC(doc *Doc, frags map[build.Key]builtins.Fragment, docPath string) ([]byte, error) {
	// A heading holds no block element, so there is no include to draw.
	return newRenderer(doc, frags, docPath, nil).renderTOC()
}

// RenderSummary renders the summary doc carries in its metadata, or "" if it
// carries none. It is a fragment of a document rather than one of its own, so
// it is rendered against the site root and without the endnote list a whole
// page gets.
func RenderSummary(doc *Doc, frags map[build.Key]builtins.Fragment) (string, error) {
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
// highlighted to, and what draws the includes. includes is nil for a render
// that can hold none.
type renderer struct {
	path     string
	includes Includes
	doc      *value.Document
	index    *value.Index
	frags    map[build.Key]builtins.Fragment
	notes    notes
	cited    []value.Content
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

func newRenderer(doc *Doc, frags map[build.Key]builtins.Fragment, docPath string, inc Includes) *renderer {
	return &renderer{
		path:     docPath,
		includes: inc,
		doc:      doc.Doc,
		index:    doc.Index,
		frags:    frags,
		notes:    indexNotes(doc.Doc),
	}
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
		// that was computed for it, and the fragment says which hook draws it.
		p, ok := c.Value.(*builtins.Include)
		if !ok {
			return true, fmt.Errorf("unsupported custom payload: %T", c.Value)
		}
		if r.includes == nil {
			return true, fmt.Errorf("%s cannot be rendered here", c.Elem)
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
			return true, r.includes.Snippet(e, p, lines)
		case builtins.Edits:
			return true, r.includes.Diff(e, p, f)
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
