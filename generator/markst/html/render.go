// Package html renders a compiled markst document as the HTML the site serves.
//
// The rendering itself is znkr.io/markst/html's job. What is here is the part
// that belongs to this site: the syntax highlighting raw text gets, the
// fragment templates the include elements are drawn with, the anchor link on
// every heading, the site-relative resolution of image paths, and the table of
// contents the article template puts beside the body.
package html

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"path"
	"strings"

	"flo.znkr.io/generator/highlight"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/site"
	"znkr.io/markst"
	mhtml "znkr.io/markst/html"
	"znkr.io/markst/value"
)

// RenderData is what a [Renderer] renders, held in [site.Doc.RenderData].
type RenderData struct {
	Doc *value.Document
}

type Renderer struct {
	page          *template.Template
	snippet, diff *template.Template
}

type Options struct {
	PageTemplate string
}

func NewRenderer(templates *template.Template, opts Options) (*Renderer, error) {
	page := templates.Lookup(opts.PageTemplate)
	if page == nil {
		return nil, fmt.Errorf("template not found %s", opts.PageTemplate)
	}

	snippet := templates.Lookup("fragments/include_snippet")
	if snippet == nil {
		return nil, fmt.Errorf("template not found fragments/include_snippet")
	}
	diff := templates.Lookup("fragments/include_diff")
	if diff == nil {
		return nil, fmt.Errorf("template not found fragments/include_diff")
	}

	return &Renderer{
		page:    page,
		snippet: snippet,
		diff:    diff,
	}, nil
}

func (r *Renderer) RenderContent(s *site.Site, doc *site.Doc) ([]byte, error) {
	content, _, err := r.renderContent(doc)
	return content, err
}

func (r *Renderer) RenderPage(s *site.Site, doc *site.Doc) ([]byte, error) {
	content, toc, err := r.renderContent(doc)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = r.page.Execute(&buf, struct {
		Meta    *site.Metadata
		Site    *site.Site
		Content template.HTML
		TOC     template.HTML
	}{
		Meta:    doc.Meta,
		Site:    s,
		Content: template.HTML(content),
		TOC:     template.HTML(toc),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}

// RenderSummary renders the summary a document carries in its metadata. It is a
// fragment of a document rather than one of its own, so it is rendered against
// the site root and without the endnote list a whole page gets.
func RenderSummary(c value.Content) (string, error) {
	var buf strings.Builder
	opts := (&renderer{path: "/"}).options()
	err := mhtml.Render(&buf, c, append(opts, mhtml.WithoutFootnoteList())...)
	return buf.String(), err
}

func (r *Renderer) renderContent(doc *site.Doc) (content []byte, toc []byte, err error) {
	d := doc.RenderData.(*RenderData).Doc
	rr := &renderer{path: doc.Path, snippet: r.snippet, diff: r.diff}

	var buf bytes.Buffer
	if err := mhtml.Render(&buf, d, rr.options()...); err != nil {
		return nil, nil, err
	}

	toc, err = rr.renderTOC(d)
	if err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), toc, nil
}

// renderer holds what the site brings to a render: where the document sits, so
// that the paths in it resolve, and the fragments the include elements are
// drawn with.
type renderer struct {
	path          string
	snippet, diff *template.Template
}

// options is how the site's rendering differs from the default one.
func (r *renderer) options() []mhtml.Option {
	return []mhtml.Option{
		// <h1> is the article title, written by the page template.
		mhtml.WithHeadingLevel(2),
		mhtml.WithImageURL(func(p string) (string, error) { return path.Join(r.path, p), nil }),
		mhtml.WithElement(r.render),
	}
}

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
		// Highlighted the same way include-snippet highlights the files it
		// reads, so that the two look alike.
		code, err := highlightRaw(c)
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
		// builtins.Bindings. The payload is the finished table, so all that is
		// left is to hand it to the fragment that renders it.
		switch p := c.Value.(type) {
		case *builtins.SnippetData:
			return true, r.execute(e, r.snippet, p)
		case *builtins.DiffData:
			return true, r.execute(e, r.diff, p)
		default:
			return true, fmt.Errorf("unsupported custom payload: %T", p)
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
		// RenderSummary walks a fragment of a document with no templates to
		// hand; an include in a summary is a mistake worth saying out loud.
		return fmt.Errorf("%T cannot be rendered here: no templates", data)
	}
	if err := t.Execute(e, data); err != nil {
		return fmt.Errorf("rendering %s: %v", t.Name(), err)
	}
	return nil
}

// highlightRaw highlights a raw node. The lexer comes from the node's language;
// without one -- or with one chroma doesn't know -- the fallback lexer emits
// the text unstyled, which is what a raw node without a language should look
// like.
func highlightRaw(v *value.Raw) (string, error) {
	var opt highlight.Option
	if v.Lang != "" {
		opt = highlight.Lang(v.Lang)
	}
	lines, err := highlight.Highlight(v.Text, opt)
	if err != nil {
		return "", fmt.Errorf("highlighting raw %s: %v", rawKind(v), err)
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

// renderTOC renders the document's headings as a nested <ul> list, each entry
// linking to its heading. It fails if a heading has no label, and with it no
// anchor to link to.
func (r *renderer) renderTOC(d *value.Document) ([]byte, error) {
	var buf bytes.Buffer
	// The heading bodies are rendered as fragments of the document they came
	// from, so a footnote cited in one keeps the number it has in the body
	// rather than starting a list of its own.
	opts := append(r.options(),
		mhtml.WithFootnotes(mhtml.Footnotes(d)),
		mhtml.WithoutFootnoteList(),
	)
	if err := r.writeTOC(&buf, markst.Outline(d), opts); err != nil {
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
