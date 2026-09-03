package html_test

import (
	"html/template"
	"os"
	"strings"
	"testing"

	gmarkst "flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/html"
	"flo.znkr.io/generator/site"
	"znkr.io/markst"
)

// render compiles src against the site's own markst library and renders it the
// way a page body is rendered. The real lib/lib.mst is used rather than a stub:
// what is being checked is that the admonitions the site actually ships produce
// the markup its stylesheet is written against.
func render(t *testing.T, src string) string {
	t.Helper()

	libSrc, err := os.ReadFile("../../../lib/lib.mst")
	if err != nil {
		t.Fatalf("reading lib.mst: %v", err)
	}
	lib, diags, err := markst.CompileLibrary(t.Context(), "lib.mst", libSrc)
	if err != nil {
		t.Fatalf("compiling lib.mst: %v", err)
	}
	if len(diags) > 0 {
		t.Errorf("lib.mst has diagnostics:\n%s", formatDiags(diags))
	}

	doc, diags, err := markst.Compile(t.Context(), []byte(src), markst.WithName("test.mst"), markst.WithLibrary(lib))
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}
	if len(diags) > 0 {
		t.Errorf("document has diagnostics:\n%s", formatDiags(diags))
	}

	out, err := html.RenderSummary(doc.Body)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return out
}

// renderBody compiles src as a whole document and renders it the way a page
// is rendered: with the footnote list, which the summary path leaves out.
func renderBody(t *testing.T, src string) string {
	t.Helper()

	templates := template.New("")
	for _, name := range []string{"article", "fragments/include_snippet", "fragments/include_diff"} {
		template.Must(templates.New(name).Parse(""))
	}
	r, err := html.NewRenderer(templates, html.Options{PageTemplate: "article"})
	if err != nil {
		t.Fatalf("creating renderer: %v", err)
	}

	libSrc, err := os.ReadFile("../../../lib/lib.mst")
	if err != nil {
		t.Fatalf("reading lib.mst: %v", err)
	}
	lib, err := gmarkst.LoadLibrary(t.Context(), "lib.mst", libSrc, nil)
	if err != nil {
		t.Fatalf("compiling lib.mst: %v", err)
	}

	src = "#article(title: \"T\")\n\n" + src
	_, rd, err := gmarkst.Load(t.Context(), "test.mst", []byte(src), nil, []*gmarkst.Library{lib})
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}

	out, err := r.RenderContent(nil, &site.Doc{Path: "/test", RenderData: rd})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return string(out)
}

func formatDiags(diags []markst.Diagnostic) string {
	var sb strings.Builder
	markst.FormatDiagnostics(&sb, diags)
	return sb.String()
}

func TestAdmonitions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{{
		name: "note_default_title",
		in:   "#note[Body.]",
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		name: "tip_default_title",
		in:   "#tip[Body.]",
		want: `<div class="admonition tip"><p class="admonition-title">Tip</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		name: "note_with_title",
		in:   `#note(title: "Beware")[Body.]`,
		want: `<div class="admonition note"><p class="admonition-title">Beware</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		// The title is content, so it is escaped like any other text.
		name: "title_is_escaped",
		in:   `#note(title: "a < b & c")[Body.]`,
		want: `<div class="admonition note"><p class="admonition-title">a &lt; b &amp; c</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		// The body is realized content: markup inside it still works, and it
		// is not double-wrapped in a paragraph.
		name: "body_keeps_its_markup",
		in:   `#note[Body with *strong* and a #link("https://example.com")[link].]`,
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>Body with <strong>strong</strong> and a <a href="https://example.com">link</a>.</p>` + "\n</div>\n",
	}, {
		// A flow body groups into paragraphs, so a blank line makes two — the
		// thing a leaf element could never do.
		name: "multiple_paragraphs",
		in:   "#note[First.\n\n  Second.]",
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>First.</p>` + "\n<p>Second.</p>" + "\n</div>\n",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(t, tt.in); got != tt.want {
				t.Errorf("render(%q):\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// The rendering itself is znkr.io/markst/html's, and tested there. What is
// left here is the part this site adds to it: the anchor link on a heading, the
// highlighting raw text gets, and the table of contents.

func TestHeadingAnchorLink(t *testing.T) {
	// <h1> is the article title, so the document's own headings start at <h2>,
	// and each carries a link to itself for the stylesheet to reveal on hover.
	got := renderBody(t, "= Intro <intro>\n\n== Deeper\n")
	want := "<h2 id=\"intro\">Intro<a href=\"#intro\" class=\"anchor-link\"></a></h2>\n" +
		"<h3 id=\"deeper\">Deeper<a href=\"#deeper\" class=\"anchor-link\"></a></h3>\n"
	if got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}

func TestRawIsHighlighted(t *testing.T) {
	// A language chroma knows is marked up with this site's own classes; one it
	// does not know falls back to unstyled text.
	got := render(t, "```go\nfunc f() {}\n```")
	if !strings.Contains(got, "<pre><code>") {
		t.Errorf("render() = %q, want a highlighted code block", got)
	}
	if !strings.Contains(got, "hl-") {
		t.Errorf("render() = %q, want this site's highlight classes", got)
	}
	if plain := render(t, "```\nfunc f() {}\n```"); plain != "<pre><code>func f() {}</code></pre>\n" {
		t.Errorf("render() without a language = %q, want it unstyled", plain)
	}
}

func TestReferenceToAHeading(t *testing.T) {
	got := renderBody(t, "= Intro <intro>\n\nSee @intro.")
	want := "<h2 id=\"intro\">Intro<a href=\"#intro\" class=\"anchor-link\"></a></h2>\n" +
		`<p>See <a href="#intro">intro</a>.</p>` + "\n"
	if got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}

func TestImagePathIsSiteRelative(t *testing.T) {
	// A path in a document resolves against the page the document is served
	// at, not against the site root.
	got := renderBody(t, `#image("cat.png")`)
	if want := `<p><img src="/test/cat.png" alt=""></p>` + "\n"; got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}
